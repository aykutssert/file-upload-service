export class Semaphore {
  private readonly limit: number
  private queue: Array<() => void> = []
  private running = 0

  constructor(limit: number) {
    this.limit = limit
  }

  async acquire(): Promise<void> {
    if (this.running < this.limit) {
      this.running++
      return
    }
    await new Promise<void>(resolve => this.queue.push(resolve))
    this.running++
  }

  release(): void {
    this.running--
    const next = this.queue.shift()
    if (next) next()
  }

  async run<T>(fn: () => Promise<T>): Promise<T> {
    await this.acquire()
    try {
      return await fn()
    } finally {
      this.release()
    }
  }
}
