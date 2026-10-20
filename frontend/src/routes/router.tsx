import type { ReactElement } from 'react';
import { type RouteObject, createBrowserRouter, useLocation } from 'react-router';

import { PublicFrame } from '../components/frame/PublicFrame.tsx';
import { RouteFocus } from '../components/frame/RouteFocus.tsx';
import { SignedInFrame } from '../components/frame/SignedInFrame.tsx';
import { PlaceholderPage } from '../components/ui/Page.tsx';
import { HomePage } from '../features/home/HomePage.tsx';
import type { RouteHandle } from '../app/handles.ts';
import { NotFoundPage } from './NotFoundPage.tsx';
import { type Role, frameRoleState } from '../app/sections.ts';

function page(path: string, title: RouteHandle['title'], element: ReactElement = <PlaceholderPage />): RouteObject {
  const handle: RouteHandle = { title };
  return { path, element, handle };
}

function SharedFrame(): ReactElement {
  const state: unknown = useLocation().state;
  const role: Role = frameRoleState(state) ?? 'tutor';
  return <SignedInFrame frameRole={role} />;
}

const frames: RouteObject[] = [
  {
    element: <PublicFrame />,
    children: [
      page('/', 'page.signIn'),
      page('/signup', 'page.signUp'),
      page('/signup/tutor', 'page.signUpTutor'),
      page('/signup/student', 'page.signUpStudent'),
      page('/verify-email', 'page.verifyEmail'),
      page('/forgot-password', 'page.forgotPassword'),
      page('/reset-password', 'page.resetPassword'),
      page('/invite/:token', 'page.invitation'),
      page('/unsubscribe', 'page.unsubscribe'),
      page('*', 'page.notFound', <NotFoundPage />),
    ],
  },
  {
    element: <SignedInFrame frameRole="tutor" />,
    children: [
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
    ],
  },
  {
    element: <SignedInFrame frameRole="student" />,
    children: [
      page('/home', 'page.home', <HomePage />),
      page('/homework', 'page.homework'),
      page('/homework/:submissionId', 'page.assignment'),
      page('/tutors', 'page.tutors'),
    ],
  },
  {
    element: <SharedFrame />,
    children: [
      page('/schedule', 'page.schedule'),
      page('/lessons/:lessonId', 'page.lesson'),
      page('/messages', 'page.messages'),
      page('/messages/:channelId', 'page.messages'),
      page('/notifications', 'page.notifications'),
      page('/settings/profile', 'page.settingsProfile'),
      page('/settings/account', 'page.settingsAccount'),
      page('/settings/notifications', 'page.settingsNotifications'),
    ],
  },
];

export const routes: RouteObject[] = [{ element: <RouteFocus />, children: frames }];

export function createRouter(): ReturnType<typeof createBrowserRouter> {
  return createBrowserRouter(routes);
}
