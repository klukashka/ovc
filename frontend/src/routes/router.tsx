import type { ReactElement } from 'react';
import { type RouteObject, createBrowserRouter } from 'react-router';

import { RouteFocus } from '../components/frame/RouteFocus.tsx';
import { PlaceholderPage } from '../components/ui/Page.tsx';
import { ForgotPasswordPage, PrivacyPage, ResetPasswordPage } from '../features/auth/PasswordPages.tsx';
import { SignInPage } from '../features/auth/SignInPage.tsx';
import { SignUpChoicePage, StudentSignUpPage, TutorSignUpPage } from '../features/auth/SignUpPages.tsx';
import { VerifyEmailPage } from '../features/auth/VerifyEmailPage.tsx';
import { HomePage } from '../features/home/HomePage.tsx';
import type { RouteHandle } from '../app/handles.ts';
import { Gate } from './Gate.tsx';
import { NotFoundPage } from './NotFoundPage.tsx';

function page(path: string, title: RouteHandle['title'], element: ReactElement = <PlaceholderPage />): RouteObject {
  const handle: RouteHandle = { title };
  return { path, element, handle };
}

export const tutorPages: RouteObject[] = [
  page('/today', 'page.today', <HomePage />),
  page('/students', 'page.students'),
  page('/students/:studentId', 'page.student'),
  page('/groups', 'page.groups'),
  page('/groups/:groupId', 'page.group'),
  page('/homework/library', 'page.library'),
  page('/homework/templates/:templateId', 'page.builder'),
  page('/homework/issued', 'page.issued'),
  page('/homework/issued/:issueId', 'page.issue'),
  page('/homework/review', 'page.review'),
  page('/homework/review/:submissionId', 'page.reviewSubmission'),
  page('/settings/teaching', 'page.settingsTeaching'),
];

export const studentPages: RouteObject[] = [
  page('/home', 'page.home', <HomePage />),
  page('/homework', 'page.homework'),
  page('/homework/:submissionId', 'page.assignment'),
  page('/tutors', 'page.tutors'),
];

export const sharedPages: RouteObject[] = [
  page('/schedule', 'page.schedule'),
  page('/lessons/:lessonId', 'page.lesson'),
  page('/messages', 'page.messages'),
  page('/messages/:channelId', 'page.messages'),
  page('/notifications', 'page.notifications'),
  page('/settings/profile', 'page.settingsProfile'),
  page('/settings/account', 'page.settingsAccount'),
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
      page('/invite/:token', 'page.invitation'),
      page('/unsubscribe', 'page.unsubscribe'),
    ],
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
