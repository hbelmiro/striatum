export const meta = {
  name: 'example-review',
  description: 'Example code review workflow',
  phases: [
    { title: 'Review', detail: 'Review changed files' },
  ],
}

phase('Review')
const result = await agent('Review the recent changes for correctness and style issues.', {
  label: 'review',
  phase: 'Review',
})
return { review: result }
