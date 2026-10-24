import type { ReactElement } from 'react';
import { type RouteObject, createBrowserRouter } from 'react-router';

import { RouteFocus } from '../components/frame/RouteFocus.tsx';
import { PlaceholderPage } from '../components/ui/Page.tsx';
import { InvitationPage } from '../features/auth/InvitationPage.tsx';
import { InvitationTokenHolder } from '../features/auth/InvitationToken.tsx';
import { ForgotPasswordPage, PrivacyPage, ResetPasswordPage } from '../features/auth/PasswordPages.tsx';
import { ReclaimPage } from '../features/auth/ReclaimPage.tsx';
import { SignInPage } from '../features/auth/SignInPage.tsx';
import { SignUpChoicePage, StudentSignUpPage, TutorSignUpPage } from '../features/auth/SignUpPages.tsx';
import { VerifyEmailPage } from '../features/auth/VerifyEmailPage.tsx';
import { AccountSettingsPage } from '../features/settings/AccountSettingsPage.tsx';
import { ProfileSettingsPage } from '../features/settings/ProfileSettingsPage.tsx';
import { TeachingSettingsPage } from '../features/settings/TeachingSettingsPage.tsx';
import { StudentCardPage } from '../features/students/StudentCardPage.tsx';
import { StudentsPage } from '../features/students/StudentsPage.tsx';
import { HomePage } from '../features/home/HomePage.tsx';
import { MyTutorsPage } from '../features/tutors/MyTutorsPage.tsx';
import type { RouteHandle } from '../app/handles.ts';
import { Gate } from './Gate.tsx';
import { NotFoundPage } from './NotFoundPage.tsx';

function page(path: string, title: RouteHandle['title'], element: ReactElement = <PlaceholderPage />): RouteObject {
  const handle: RouteHandle = { title };
  return { path, element, handle };
}

export const tutorPages: RouteObject[] = [
  page('/today', 'page.today', <HomePage />),
  page('/students', 'page.students', <StudentsPage />),
  page('/students/:studentId', 'page.student', <StudentCardPage />),
  page('/groups', 'page.groups'),
  page('/groups/:groupId', 'page.group'),
  page('/homework/library', 'page.library'),
  page('/homework/templates/:templateId', 'page.builder'),
  page('/homework/issued', 'page.issued'),
  page('/homework/issued/:issueId', 'page.issue'),
  page('/homework/review', 'page.review'),
  page('/homework/review/:submissionId', 'page.reviewSubmission'),
  page('/settings/teaching', 'page.settingsTeaching', <TeachingSettingsPage />),
];

export const studentPages: RouteObject[] = [
  page('/home', 'page.home', <HomePage />),
  page('/homework', 'page.homework'),
  page('/homework/:submissionId', 'page.assignment'),
  page('/tutors', 'page.tutors', <MyTutorsPage />),
];

export const sharedPages: RouteObject[] = [
  page('/schedule', 'page.schedule'),
  page('/lessons/:lessonId', 'page.lesson'),
  page('/messages', 'page.messages'),
  page('/messages/:channelId', 'page.messages'),
  page('/notifications', 'page.notifications'),
  page('/settings/profile', 'page.settingsProfile', <ProfileSettingsPage />),
  page('/settings/account', 'page.settingsAccount', <AccountSettingsPage />),
  page('/settings/notifications', 'page.settingsNotifications'),
];

const frames: RouteObject[] = [
  {
    element: <Gate audience="guest" />,
    children: [
      page('/', 'page.signIn', <SignInPage />),
      page('/signup', 'page.signUp', <SignUpChoicePage />),
      page('/signup/tutor', 'page.signUpTutor', <TutorSignUpPage />),
      page('/signup/student', 'page.signUpStudent', <StudentSignUpPage />),
    ],
  },
  {
    element: <Gate audience="public" />,
    children: [
      page('/verify-email', 'page.verifyEmail', <VerifyEmailPage />),
      page('/forgot-password', 'page.forgotPassword', <ForgotPasswordPage />),
      page('/reset-password', 'page.resetPassword', <ResetPasswordPage />),
      page('/privacy', 'page.privacy', <PrivacyPage />),
      page('/unsubscribe', 'page.unsubscribe'),
    ],
  },
  {
    path: '/invite/:token?',
    element: <InvitationTokenHolder />,
    children: [
      {
        element: <Gate audience="public" />,
        children: [
          { index: true, element: <InvitationPage />, handle: { title: 'page.invitation' } satisfies RouteHandle },
        ],
      },
    ],
  },
  { element: <Gate audience="reclaim" />, children: [page('/reclaim', 'page.reclaim', <ReclaimPage />)] },
  { element: <Gate audience="tutor" />, children: tutorPages },
  { element: <Gate audience="student" />, children: studentPages },
  { element: <Gate audience="shared" />, children: sharedPages },
  { element: <Gate audience="unmatched" />, children: [page('*', 'page.notFound', <NotFoundPage />)] },
];

export const routes: RouteObject[] = [{ element: <RouteFocus />, children: frames }];

export function createRouter(): ReturnType<typeof createBrowserRouter> {
  return createBrowserRouter(routes);
}
