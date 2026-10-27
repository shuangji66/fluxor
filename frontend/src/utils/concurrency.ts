/**
 * 受限并发地执行异步任务，结果顺序与输入一致。
 *
 * 用于「逐组测速」这类既不能串行（组多时太慢）又不宜全量并发（会同时打满内核）的场景。
 * 用固定数量的 worker 从队首取任务，避免一次性 Promise.all 创建 N 个并发请求。
 *
 * 单个任务抛错不会中断整批：对应位置返回 null，由调用方决定如何统计失败。
 */
export async function mapWithConcurrency<T, R>(
  items: readonly T[],
  limit: number,
  task: (item: T, index: number) => Promise<R>
): Promise<(R | null)[]> {
  const results: (R | null)[] = new Array(items.length).fill(null)
  const workers = Math.max(1, Math.min(limit, items.length))
  let next = 0

  const run = async () => {
    while (true) {
      const index = next++
      if (index >= items.length) return
      try {
        results[index] = await task(items[index], index)
      } catch {
        results[index] = null
      }
    }
  }

  await Promise.all(Array.from({ length: workers }, run))
  return results
}
