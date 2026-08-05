type AttemptWindow = {
  count: number;
  startedAt: number;
};

export class LoginLimiter {
  private readonly attempts = new Map<string, AttemptWindow>();

  constructor(
    private readonly limit = 5,
    private readonly windowMs = 60_000,
    private readonly maxKeys = 1024,
  ) {}

  allow(key: string, now = Date.now()) {
    const current = this.attempts.get(key);
    if (!current || now - current.startedAt >= this.windowMs) {
      this.remember(key, { count: 1, startedAt: now });
      return true;
    }
    if (current.count >= this.limit) return false;

    current.count += 1;
    return true;
  }

  private remember(key: string, window: AttemptWindow) {
    if (!this.attempts.has(key) && this.attempts.size >= this.maxKeys) {
      const oldest = this.attempts.keys().next().value;
      if (oldest !== undefined) this.attempts.delete(oldest);
    }
    this.attempts.set(key, window);
  }
}
