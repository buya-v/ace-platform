import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { DailyScheduler } from '../daily-admin.js';

// Mock node-cron
vi.mock('node-cron', () => {
  const tasks: Array<{ cronExpr: string; callback: () => void; stopped: boolean }> = [];

  return {
    default: {
      schedule: vi.fn((cronExpr: string, callback: () => void) => {
        const task = { cronExpr, callback, stopped: false };
        tasks.push(task);
        return {
          stop: vi.fn(() => {
            task.stopped = true;
          }),
        };
      }),
    },
    __tasks: tasks,
  };
});

describe('DailyScheduler', () => {
  let consoleSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    consoleSpy = vi.spyOn(console, 'log').mockImplementation(() => {});
    vi.clearAllMocks();
  });

  afterEach(() => {
    consoleSpy.mockRestore();
  });

  it('constructs with default options', () => {
    const scheduler = new DailyScheduler();
    expect(scheduler).toBeDefined();
  });

  it('constructs with custom hours', () => {
    const scheduler = new DailyScheduler({
      morningHour: 7,
      eodHour: 18,
      agingTicketsHour: 11,
    });
    expect(scheduler).toBeDefined();
  });

  it('starts and registers three cron jobs', async () => {
    const cron = await import('node-cron');
    const scheduler = new DailyScheduler();
    scheduler.start();

    expect(cron.default.schedule).toHaveBeenCalledTimes(3);
  });

  it('uses custom morning hour in cron expression', async () => {
    const cron = await import('node-cron');
    const scheduler = new DailyScheduler({ morningHour: 6 });
    scheduler.start();

    // First call is morning health check
    expect(cron.default.schedule).toHaveBeenCalledWith(
      '0 6 * * *',
      expect.any(Function),
    );
  });

  it('uses custom eod hour in cron expression', async () => {
    const cron = await import('node-cron');
    const scheduler = new DailyScheduler({ eodHour: 18 });
    scheduler.start();

    // Second call is EOD report (weekdays only)
    expect(cron.default.schedule).toHaveBeenCalledWith(
      '0 18 * * 1-5',
      expect.any(Function),
    );
  });

  it('uses custom aging tickets hour in cron expression', async () => {
    const cron = await import('node-cron');
    const scheduler = new DailyScheduler({ agingTicketsHour: 11 });
    scheduler.start();

    // Third call is aging tickets check
    expect(cron.default.schedule).toHaveBeenCalledWith(
      '0 11 * * *',
      expect.any(Function),
    );
  });

  it('uses default hours (9, 17, 10) when no options', async () => {
    const cron = await import('node-cron');
    const scheduler = new DailyScheduler();
    scheduler.start();

    const calls = (cron.default.schedule as ReturnType<typeof vi.fn>).mock.calls;
    expect(calls[0][0]).toBe('0 9 * * *');      // morning at 9
    expect(calls[1][0]).toBe('0 17 * * 1-5');   // eod at 17, weekdays
    expect(calls[2][0]).toBe('0 10 * * *');      // aging at 10
  });

  it('stop() stops all scheduled tasks', async () => {
    const cron = await import('node-cron');
    const scheduler = new DailyScheduler();
    scheduler.start();

    const stopFns = (cron.default.schedule as ReturnType<typeof vi.fn>).mock.results.map(
      (r: { value: { stop: ReturnType<typeof vi.fn> } }) => r.value.stop,
    );

    scheduler.stop();

    for (const stopFn of stopFns) {
      expect(stopFn).toHaveBeenCalled();
    }
  });

  it('stop() clears internal task list so restart registers new tasks', async () => {
    const cron = await import('node-cron');
    const scheduler = new DailyScheduler();

    scheduler.start();
    expect(cron.default.schedule).toHaveBeenCalledTimes(3);

    scheduler.stop();

    // Clear the mock call count
    (cron.default.schedule as ReturnType<typeof vi.fn>).mockClear();

    scheduler.start();
    expect(cron.default.schedule).toHaveBeenCalledTimes(3);
  });

  it('logs start message', () => {
    const scheduler = new DailyScheduler();
    scheduler.start();

    expect(consoleSpy).toHaveBeenCalledWith(
      expect.stringContaining('[daily-admin:scheduler]'),
    );
  });

  it('logs stop message', () => {
    const scheduler = new DailyScheduler();
    scheduler.start();
    scheduler.stop();

    expect(consoleSpy).toHaveBeenCalledWith(
      expect.stringContaining('Stopped all scheduled tasks'),
    );
  });
});
