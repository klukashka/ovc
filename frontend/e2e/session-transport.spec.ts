import {
  type ContractSubject,
  type SeededDeadline,
  type SeededLessons,
  type SeededSharedStudent,
  type SeededStudentHome,
  type SeededSubmission,
  authTransportContract,
} from '../src/lib/api/transportContract.ts';
import { httpAuthTransport } from '../src/lib/api/transport.ts';
import { type TestAccount, newTestAccount, registerTutor, testNetAddress } from './accounts.ts';
import { createPastSimpleTemplate, onlySubmission, questionIds } from './assignment.ts';
import type { Deployment } from './deployment.ts';
import { expect, seedVerifiedUser, test } from './fixtures.ts';
import { issueThroughTheApi } from './homework.ts';
import { publishReview } from './results.ts';
import { answerTextAndSubmit } from './review.ts';
import { addDays, localDate, scheduleLessons } from './schedule.ts';
import { acceptSignedIn, createCard, endRelationship, seedActiveStudent } from './tutoring.ts';
import { unsubscribeTokenFromAReminder } from './unsubscribe.ts';

const refreshCookie = '__Host-ovc_refresh';
const dayMilliseconds = 24 * 60 * 60 * 1_000;

interface SeededWork extends SeededSubmission {
  readonly student: TestAccount;
}

async function seedWork(deployment: Deployment, tutor: TestAccount): Promise<SeededWork> {
  const { student, cardId } = await seedActiveStudent(deployment, tutor, 'Contract student');
  const templateId = await createPastSimpleTemplate(deployment, tutor);
  await issueThroughTheApi(deployment, tutor, templateId, { relationship_id: cardId });
  const submissionId = await onlySubmission(deployment, student);
  const [, textQuestionId, fileQuestionId] = await questionIds(deployment, student, submissionId);
  if (textQuestionId === undefined || fileQuestionId === undefined) {
    throw new Error('the seeded submission has no text and file questions');
  }
  return {
    email: student.email,
    password: student.password,
    submissionId,
    textQuestionId,
    fileQuestionId,
    relationshipId: cardId,
    student,
  };
}

async function linkExistingStudent(deployment: Deployment, tutor: TestAccount, student: TestAccount): Promise<string> {
  const card = await createCard(deployment, tutor, student.name, student.email);
  await acceptSignedIn(deployment, student, card.link);
  return card.id;
}

function cookieJarFetch(deployment: Deployment): typeof fetch {
  let cookie: string | null = null;
  const address = testNetAddress();
  return async (input, init) => {
    const request = new Request(input, init);
    const headers = new Headers(request.headers);
    headers.set('Origin', deployment.origin);
    headers.set('X-Forwarded-For', address);
    if (cookie !== null) {
      headers.set('Cookie', `${refreshCookie}=${cookie}`);
    }
    const response = await fetch(new Request(request, { headers, signal: request.signal }));
    for (const setCookie of response.headers.getSetCookie()) {
      const [pair = '', ...attributes] = setCookie.split(';').map((part) => part.trim());
      const separator = pair.indexOf('=');
      const name = separator < 0 ? pair : pair.slice(0, separator);
      const value = separator < 0 ? '' : pair.slice(separator + 1);
      if (name !== refreshCookie) {
        continue;
      }
      const cleared = value === '' || attributes.some((attribute) => /^max-age=0$/i.test(attribute));
      cookie = cleared ? null : value;
    }
    return response;
  };
}

test.describe('the HTTP authentication transport against the real backend', () => {
  for (const [name, scenario] of Object.entries(authTransportContract)) {
    test(`keeps the contract: ${name}`, async ({ deployment, control }) => {
      const account = await seedVerifiedUser(deployment, 'Contract tutor');
      const otherTutor = await seedVerifiedUser(deployment, 'Another contract tutor');
      const subject: ContractSubject = {
        transport: httpAuthTransport(deployment.apiUrl, cookieJarFetch(deployment)),
        origin: deployment.apiUrl,
        email: account.email,
        password: account.password,
        otherTutorEmail: otherTutor.email,
        otherTutorPassword: otherTutor.password,
        student: async (): Promise<{ readonly email: string; readonly password: string }> => {
          const { student } = await seedActiveStudent(deployment, otherTutor, 'Contract student');
          return { email: student.email, password: student.password };
        },
        expireAccessTokens: (): Promise<void> => control.advance(16 * 60),
        passSeconds: (seconds: number): Promise<void> => control.advance(seconds),
        blockAccount: (): Promise<void> => {
          deployment.administer(['users', 'block', account.email]);
          return Promise.resolve();
        },
        unsubscribeToken: (): Promise<string> => unsubscribeTokenFromAReminder(deployment, control, account),
        seedTwoLessons: async (): Promise<SeededLessons> => {
          const { cardId } = await seedActiveStudent(deployment, account, 'Contract student');
          const date = addDays(localDate(await control.now(), 'Europe/Moscow'), 1);
          const lesson = { relationship_id: cardId, title: 'Lesson', date, duration_minutes: 60 };
          const [laterStart] = await scheduleLessons(deployment, account, { ...lesson, start: '12:00' });
          const [earlierStart] = await scheduleLessons(deployment, account, { ...lesson, start: '09:00' });
          if (laterStart === undefined || earlierStart === undefined) {
            throw new Error('a seeded lesson has no start');
          }
          const hour = 60 * 60 * 1_000;
          return {
            from: new Date(Date.parse(earlierStart) - hour).toISOString(),
            to: new Date(Date.parse(laterStart) + 2 * hour).toISOString(),
            laterStart,
            earlierStart,
          };
        },
        seedSubmission: (): Promise<SeededSubmission> => seedWork(deployment, account),
        fillTutorFileQuota: (): Promise<void> => {
          deployment.administer(['tutors', 'set-quota', account.email, '--files', '1', '--recordings', '1']);
          return Promise.resolve();
        },
        seedUpcomingDeadline: async (): Promise<SeededDeadline> => {
          const { cardId } = await seedActiveStudent(deployment, account, 'Contract student');
          const templateId = await createPastSimpleTemplate(deployment, account);
          const deadlineAt = new Date((await control.now()) + 2 * dayMilliseconds).toISOString();
          const audience = { relationship_id: cardId };
          return { issueId: await issueThroughTheApi(deployment, account, templateId, audience, { deadlineAt }) };
        },
        seedStudentHome: async (): Promise<SeededStudentHome> => {
          const work = await seedWork(deployment, account);
          await answerTextAndSubmit(deployment, work.student, work.submissionId, 'went');
          await publishReview(deployment, account, work.submissionId, 'check', 'Well done.', [
            { questionId: work.fileQuestionId, points: 4 },
          ]);
          const templateId = await createPastSimpleTemplate(deployment, account);
          await issueThroughTheApi(deployment, account, templateId, { relationship_id: work.relationshipId });
          const date = addDays(localDate(await control.now(), 'Europe/Moscow'), 1);
          await scheduleLessons(deployment, account, {
            relationship_id: work.relationshipId,
            title: 'Lesson',
            date,
            start: '12:00',
            duration_minutes: 60,
          });
          const endedRelationshipId = await linkExistingStudent(deployment, otherTutor, work.student);
          await endRelationship(deployment, otherTutor, endedRelationshipId);
          const otherStudent = await seedActiveStudent(deployment, account, 'Another contract student');
          return {
            email: work.email,
            password: work.password,
            relationshipId: work.relationshipId,
            endedRelationshipId,
            otherStudentRelationshipId: otherStudent.cardId,
          };
        },
        unconfirmedTutor: async (): Promise<{ readonly email: string; readonly password: string }> => {
          const tutor = newTestAccount('Unconfirmed contract tutor');
          await registerTutor(deployment, tutor, 'en');
          return { email: tutor.email, password: tutor.password };
        },
        seedSharedStudentIssues: async (): Promise<SeededSharedStudent> => {
          const { student, cardId } = await seedActiveStudent(deployment, account, 'Shared contract student');
          const otherTutorCardId = await linkExistingStudent(deployment, otherTutor, student);
          const ownTemplate = await createPastSimpleTemplate(deployment, account);
          await issueThroughTheApi(deployment, account, ownTemplate, { relationship_id: cardId });
          const otherTemplate = await createPastSimpleTemplate(deployment, otherTutor);
          await issueThroughTheApi(deployment, otherTutor, otherTemplate, { relationship_id: otherTutorCardId });
          await issueThroughTheApi(deployment, otherTutor, otherTemplate, { relationship_id: otherTutorCardId });
          return { cardId, otherTutorCardId };
        },
        seedCheckedSubmission: async (): Promise<SeededSubmission> => {
          const work = await seedWork(deployment, account);
          await answerTextAndSubmit(deployment, work.student, work.submissionId, 'went');
          await publishReview(deployment, account, work.submissionId, 'check', 'Well done.', [
            { questionId: work.fileQuestionId, points: 4 },
          ]);
          return work;
        },
        seedOtherStudentSubmission: (): Promise<SeededSubmission> => seedWork(deployment, account),
        returnSeededSubmission: (seeded: SeededSubmission): Promise<void> =>
          publishReview(deployment, account, seeded.submissionId, 'return', 'Please redo the photo.', [
            { questionId: seeded.fileQuestionId, comment: 'Your photo is blurred.' },
          ]),
        endSeededRelationship: (seeded: SeededSubmission): Promise<void> =>
          endRelationship(deployment, account, seeded.relationshipId),
      };

      const run = scenario(subject);

      await expect(run).resolves.toBeUndefined();
    });
  }
});
