export const meta = {
  name: 'pr-review',
  description: 'Review a pull request in two passes',
  phases: [
    { title: 'Summarize', detail: 'Summarize the changed files' },
    { title: 'Review', detail: 'Review the changes for issues' },
  ],
}

phase('Summarize')
const summary = await agent('Summarize the pull request changes in a short bullet list.', {
  label: 'summarize',
  phase: 'Summarize',
})

phase('Review')
const findings = await agent(
  'Review the changed files for correctness and style issues. Report each finding with a file and line reference.',
  { label: 'review', phase: 'Review' },
)

return { summary, findings }
