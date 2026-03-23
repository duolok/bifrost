const std = @import("std");
const proc = @import("proc.zig");
const probe = @import("probe.zig");

pub const HealthReport = struct {
    healthy: bool,
    response_time_ms: u64,
    memory_used_kb: u64,
    memory_total_kb: u64,
    cpu_usage_percent: f64,
    open_fds: u64,

    pub fn from(metrics: proc.Metrics, result: probe.ProbeResult) HealthReport {
        return .{
            .healthy = result.healthy,
            .response_time_ms = result.response_time_ms,
            .memory_used_kb = metrics.memory_used_kb,
            .memory_total_kb = metrics.memory_total_kb,
            .cpu_usage_percent = metrics.cpu_usage_percent,
            .open_fds = metrics.open_fds,
        };
    }
};

pub fn send(allocator: std.mem.Allocator, host: []const u8, port: u16, deploy_id: []const u8, report: HealthReport) void {
    doSend(allocator, host, port, deploy_id, report) catch |err| {
        std.log.warn("failed to send health report: {}", .{err});
    };
}

fn doSend(allocator: std.mem.Allocator, host: []const u8, port: u16, deploy_id: []const u8, report: HealthReport) !void {
    var body_buf: [512]u8 = undefined;
    const body = try std.fmt.bufPrint(&body_buf,
        \\{{"healthy":{},"response_time_ms":{d},"memory_used_kb":{d},"memory_total_kb":{d},"cpu_usage_percent":{d:.1},"open_fds":{d}}}
    , .{
        report.healthy,
        report.response_time_ms,
        report.memory_used_kb,
        report.memory_total_kb,
        report.cpu_usage_percent,
        report.open_fds,
    });

    var req_buf: [1024]u8 = undefined;
    const req = try std.fmt.bufPrint(&req_buf,
        "POST /api/v1/deployments/{s}/health HTTP/1.1\r\nHost: {s}\r\nContent-Type: application/json\r\nContent-Length: {d}\r\nConnection: close\r\n\r\n{s}",
        .{ deploy_id, host, body.len, body },
    );

    const stream = try std.net.tcpConnectToHost(allocator, host, port);
    defer stream.close();

    try stream.writeAll(req);

    var resp_buf: [256]u8 = undefined;
    const n = try stream.read(&resp_buf);
    if (n < 12) return error.BadResponse;

    const status = try std.fmt.parseInt(u16, resp_buf[9..12], 10);
    if (status >= 400) {
        std.log.warn("gateway returned status {d}", .{status});
    }
}
