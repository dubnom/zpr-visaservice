import { test, expect } from '@playwright/test';

const requestEvent = {
  id: 'event-1',
  organization_id: 'org-<example>',
  occurred_at: '2026-10-28T12:00:00Z',
  actor_issuer: 'https://idp.example',
  actor_subject: 'operator <img src=x>',
  action: 'budget.approval.create',
  target: '/operator/budget-authority/approvals',
  outcome: 'self-approval-requested',
  source: 'control-service',
  correlation_id: 'request-1',
};

test('budget request advisory is evidence-bound and does not claim an approval', async ({ page }) => {
  let simulatorRequests = 0;
  await page.route('**/api/simulator/**', route => {
    simulatorRequests++;
    return route.abort();
  });
  await page.route('**/api/audit/events?*', route => route.fulfill({ json: {
    events: [
      requestEvent,
      { ...requestEvent, id: 'event-2', outcome: 'requested' },
      { ...requestEvent, id: 'event-3', action: 'policy.approval.create' },
      { ...requestEvent, id: 'event-4', source: 'other-service' },
    ],
  } }));

  await page.goto('/#history');
  await page.locator('#load-live-history').click();
  await expect(page.locator('#live-history-status')).toContainText('Loaded 4 events.');
  const events = page.locator('#live-history-events .event');
  await expect(events).toHaveCount(4);
  const warning = events.nth(0).locator('.audit-warning');
  await expect(warning).toContainText('Potential self-approval authority');
  await expect(warning).toContainText('does not establish a self-approved operation, a successful decision, or a current grant');
  await expect(warning).toContainText('Organization: org-<example> · Role: unknown · Exact approval or revision: unknown');
  await expect(events.nth(0)).toContainText('https://idp.example / operator <img src=x>');
  await expect(events.nth(0)).toContainText('Event: event-1 · Correlation: request-1');
  await expect(page.locator('#live-history-events .audit-warning')).toHaveCount(1);
  await expect(page.locator('#live-history-events img')).toHaveCount(0);
  expect(simulatorRequests).toBe(0);
});

test('advisories paginate with their evidence and clear on unavailable history', async ({ page }) => {
  let requests = 0;
  await page.route('**/api/audit/events?*', route => {
    requests++;
    if (requests === 1) {
      return route.fulfill({ json: { events: [{ ...requestEvent, outcome: 'requested' }], next_cursor: 'next' } });
    }
    if (requests === 2) {
      const { organization_id, ...eventWithoutOrganization } = requestEvent;
      return route.fulfill({ json: { events: [eventWithoutOrganization] } });
    }
    return route.fulfill({ status: 503, json: { error: 'unavailable' } });
  });

  await page.goto('/#history');
  await page.locator('#load-live-history').click();
  await expect(page.locator('#live-history-events .audit-warning')).toHaveCount(0);
  await page.locator('#live-history-more').click();
  await expect(page.locator('#live-history-events .event')).toHaveCount(2);
  await expect(page.locator('#live-history-events .audit-warning')).toContainText('Organization: Not provided');
  await page.locator('#load-live-history').click();
  await expect(page.locator('#live-history-status')).toContainText('unavailable or not configured');
  await expect(page.locator('#live-history-events')).toBeEmpty();
});

test('single-operator subject-link reviews show only the supported observed evidence', async ({ page }) => {
  const review = {
    ...requestEvent,
    id: 'review-event-<1>',
    action: 'subject-link.approve',
    target: 'subject-link-review/review-<1>',
    outcome: 'approved-single-operator',
  };
  let simulatorRequests = 0;
  await page.route('**/api/simulator/**', route => {
    simulatorRequests++;
    return route.abort();
  });
  await page.route('**/api/audit/events?*', route => route.fulfill({ json: {
    events: [
      review,
      { ...review, id: 'other-outcome', outcome: 'approved' },
      { ...review, id: 'other-action', action: 'policy.approve' },
      { ...review, id: 'other-source', source: 'other-service' },
      { ...review, id: '', organization_id: '', actor_issuer: '', actor_subject: '', target: '' },
    ],
  } }));

  await page.goto('/#history');
  await page.locator('#load-live-history').click();
  const events = page.locator('#live-history-events .event');
  await expect(events).toHaveCount(5);
  const warning = events.nth(0).locator('.audit-warning');
  await expect(warning).toContainText('Observed single-operator review');
  await expect(warning).toContainText('the proposer approved this subject-link review');
  await expect(warning).toContainText('not publication or enforcement');
  await expect(warning).toContainText('may not be the original approval time');
  await expect(warning).toContainText('Organization: org-<example> · Role: unknown · Revision: unknown');
  await expect(events.nth(0)).toContainText('Actor: https://idp.example / operator <img src=x>');
  await expect(events.nth(0)).toContainText('Target: subject-link-review/review-<1>');
  await expect(events.nth(0)).toContainText('Event: review-event-<1> · Correlation: request-1');
  await expect(page.locator('#live-history-events .audit-warning')).toHaveCount(2);
  await expect(events.nth(4)).toContainText('Actor: Not provided / Not provided');
  await expect(events.nth(4)).toContainText('Target: Not provided');
  await expect(events.nth(4)).toContainText('Event: Not provided');
  await expect(events.nth(4).locator('.audit-warning')).toContainText('Organization: Not provided');
  await expect(page.locator('#live-history-events img')).toHaveCount(0);
  expect(simulatorRequests).toBe(0);
});
