import type { ReactElement } from 'react';
import { type RouteObject, createBrowserRouter } from 'react-router';

import { RouteFocus } from '../components/frame/RouteFocus.tsx';
import { PlaceholderPage } from '../components/ui/Page.tsx';
import { AddressTokenHolder } from '../features/auth/AddressToken.tsx';
import { InvitationTokenHolder } from '../features/auth/InvitationToken.tsx';
import type { RouteHandle } from '../app/handles.ts';
import { Gate } from './Gate.tsx';
import { lazyPage } from './LazyPage.tsx';
import { NotFoundPage } from './NotFoundPage.tsx';

function page(path: string, title: RouteHandle['title'], element: ReactElement = <PlaceholderPage />): RouteObject {
  const handle: RouteHandle = { title };
  return { path, element, handle };
}

const todayPage = lazyPage(async () => (await import('../features/home/TodayPage.tsx')).TodayPage);
const studentsPage = lazyPage(async () => (await import('../features/students/StudentsPage.tsx')).StudentsPage);
const studentCardPage = lazyPage(
  async () => (await import('../features/students/StudentCardPage.tsx')).StudentCardPage,
);
const groupsPage = lazyPage(async () => (await import('../features/groups/GroupsPage.tsx')).GroupsPage);
const groupPage = lazyPage(async () => (await import('../features/groups/GroupPage.tsx')).GroupPage);
const teachingSettingsPage = lazyPage(
  async () => (await import('../features/settings/TeachingSettingsPage.tsx')).TeachingSettingsPage,
);
const homePage = lazyPage(async () => (await import('../features/home/HomePage.tsx')).HomePage);
const myTutorsPage = lazyPage(async () => (await import('../features/tutors/MyTutorsPage.tsx')).MyTutorsPage);
const profileSettingsPage = lazyPage(
  async () => (await import('../features/settings/ProfileSettingsPage.tsx')).ProfileSettingsPage,
);
const accountSettingsPage = lazyPage(
  async () => (await import('../features/settings/AccountSettingsPage.tsx')).AccountSettingsPage,
);
const notificationsPage = lazyPage(
  async () => (await import('../features/notifications/NotificationsPage.tsx')).NotificationsPage,
);
const signInPage = lazyPage(async () => (await import('../features/auth/SignInPage.tsx')).SignInPage);
const signUpChoicePage = lazyPage(async () => (await import('../features/auth/SignUpPages.tsx')).SignUpChoicePage);
const tutorSignUpPage = lazyPage(async () => (await import('../features/auth/SignUpPages.tsx')).TutorSignUpPage);
const studentSignUpPage = lazyPage(async () => (await import('../features/auth/SignUpPages.tsx')).StudentSignUpPage);
const forgotPasswordPage = lazyPage(
  async () => (await import('../features/auth/PasswordPages.tsx')).ForgotPasswordPage,
);
const privacyPage = lazyPage(async () => (await import('../features/auth/PasswordPages.tsx')).PrivacyPage);
const verifyEmailPage = lazyPage(async () => (await import('../features/auth/VerifyEmailPage.tsx')).VerifyEmailPage);
const resetPasswordPage = lazyPage(async () => (await import('../features/auth/PasswordPages.tsx')).ResetPasswordPage);
const invitationPage = lazyPage(async () => (await import('../features/auth/InvitationPage.tsx')).InvitationPage);
const reclaimPage = lazyPage(async () => (await import('../features/auth/ReclaimPage.tsx')).ReclaimPage);

export const tutorPages: RouteObject[] = [
  page('/today', 'page.today', todayPage),
  page('/students', 'page.students', studentsPage),
  page('/students/:studentId', 'page.student', studentCardPage),
  page('/groups', 'page.groups', groupsPage),
  page('/groups/:groupId', 'page.group', groupPage),
  page('/homework/library', 'page.library'),
  page('/homework/templates/:templateId', 'page.builder'),
  page('/homework/issued', 'page.issued'),
  page('/homework/issued/:issueId', 'page.issue'),
  page('/homework/review', 'page.review'),
  page('/homework/review/:submissionId', 'page.reviewSubmission'),
  page('/settings/teaching', 'page.settingsTeaching', teachingSettingsPage),
];

export const studentPages: RouteObject[] = [
  page('/home', 'page.home', homePage),
  page('/homework', 'page.homework'),
  page('/homework/:submissionId', 'page.assignment'),
  page('/tutors', 'page.tutors', myTutorsPage),
];

export const sharedPages: RouteObject[] = [
  page('/schedule', 'page.schedule'),
  page('/lessons/:lessonId', 'page.lesson'),
  page('/messages', 'page.messages'),
  page('/messages/:channelId', 'page.messages'),
  page('/notifications', 'page.notifications', notificationsPage),
  page('/settings/profile', 'page.settingsProfile', profileSettingsPage),
  page('/settings/account', 'page.settingsAccount', accountSettingsPage),
  page('/settings/notifications', 'page.settingsNotifications'),
];

const frames: RouteObject[] = [
  {
    element: <Gate audience="guest" />,
    children: [
      page('/', 'page.signIn', signInPage),
      page('/signup', 'page.signUp', signUpChoicePage),
      page('/signup/tutor', 'page.signUpTutor', tutorSignUpPage),
      page('/signup/student', 'page.signUpStudent', studentSignUpPage),
    ],
  },
  {
    element: <Gate audience="public" />,
    children: [
      page('/forgot-password', 'page.forgotPassword', forgotPasswordPage),
      page('/privacy', 'page.privacy', privacyPage),
    ],
  },
  {
    element: <AddressTokenHolder />,
    children: [
      {
        element: <Gate audience="public" />,
        children: [
          page('/verify-email', 'page.verifyEmail', verifyEmailPage),
          page('/reset-password', 'page.resetPassword', resetPasswordPage),
          page('/unsubscribe', 'page.unsubscribe'),
        ],
      },
    ],
  },
  {
    path: '/invite/:token?',
    element: <InvitationTokenHolder />,
    children: [
      {
        element: <Gate audience="public" />,
        children: [
          {
            index: true,
            element: invitationPage,
            handle: { title: 'page.invitation' } satisfies RouteHandle,
          },
        ],
      },
    ],
  },
  {
    element: <Gate audience="reclaim" />,
    children: [page('/reclaim', 'page.reclaim', reclaimPage)],
  },
  { element: <Gate audience="tutor" />, children: tutorPages },
  { element: <Gate audience="student" />, children: studentPages },
  { element: <Gate audience="shared" />, children: sharedPages },
  { element: <Gate audience="unmatched" />, children: [page('*', 'page.notFound', <NotFoundPage />)] },
];

export const routes: RouteObject[] = [{ element: <RouteFocus />, children: frames }];

export function createRouter(): ReturnType<typeof createBrowserRouter> {
  return createBrowserRouter(routes);
}
