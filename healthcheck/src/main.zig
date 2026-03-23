const std = @import("std");
const proc = @import("proc.zig");
const probe = @import("probe.zig");
const reporter = @import("reporter.zig");

pub fn main() !void {
    const allocator = std.heap.page_allocator;

    const deploy_id = std.posix.getenv("BF_DEPLOY_ID") orelse "unknown";
    const probe_host = std.posix.getenv("BF_PROBE_HOST") orelse "localhost";
    const probe_port = parsePort(std.posix.getenv("BF_PROBE_PORT")) orelse 8080;
    const probe_path = std.posix.getenv("BF_PROBE_PATH") orelse "/health";
    const gw_host = std.posix.getenv("BF_GATEWAY_HOST") orelse "localhost";
    const gw_port = parsePort(std.posix.getenv("BF_GATEWAY_PORT")) orelse 8080;
    const interval_s = parseU64(std.posix.getenv("BF_INTERVAL_S")) orelse 10;

    std.log.info("healthcheck starting: deploy={s} probe={s}:{d}{s} gateway={s}:{d} interval={d}s", .{
        deploy_id, probe_host, probe_port, probe_path, gw_host, gw_port, interval_s,
    });

    while (true) {
        const metrics = proc.collect() catch |err| {
            std.log.warn("failed to collect metrics: {}", .{err});
            std.time.sleep(interval_s * std.time.ns_per_s);
            continue;
        };
        const result = probe.check(allocator, probe_host, probe_port, probe_path);
        const report = reporter.HealthReport.from(metrics, result);

        const status: []const u8 = if (report.healthy) "healthy" else "unhealthy";
        std.log.info("{s}: response={d}ms mem={d}/{d}KB cpu={d:.1}% fds={d}", .{
            status,
            report.response_time_ms,
            report.memory_used_kb,
            report.memory_total_kb,
            report.cpu_usage_percent,
            report.open_fds,
        });

        reporter.send(allocator, gw_host, gw_port, deploy_id, report);
        std.time.sleep(interval_s * std.time.ns_per_s);
    }
}

fn parsePort(val: ?[]const u8) ?u16 {
    const s = val orelse return null;
    return std.fmt.parseInt(u16, s, 10) catch null;
}

fn parseU64(val: ?[]const u8) ?u64 {
    const s = val orelse return null;
    return std.fmt.parseInt(u64, s, 10) catch null;
}
