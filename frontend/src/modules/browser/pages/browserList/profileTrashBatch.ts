// Process each selected instance once; a failure must not hide the other outcomes.
export async function runProfileTrashBatch(ids: string[], action: (id: string) => Promise<unknown>) {
  const succeededIds: string[] = []
  const failedIds: string[] = []
  for (const id of new Set(ids)) {
    try {
      await action(id)
      succeededIds.push(id)
    } catch {
      failedIds.push(id)
    }
  }
  return { succeededIds, failedIds }
}
