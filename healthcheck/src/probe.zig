const std = @import("std");

pub const ProbeResult = struct {
    healthy: bool,
    response_time_ms: u64,
};

pub fn check(allocator: std.mem.Allocator, host: []const u8, port: u16, path: []const u8) ProbeResult {
    const start = std.time.milliTimestamp();
    const healthy = doCheck(allocator, host, port, path);
    const elapsed: u64 = @intCast(std.time.milliTimestamp() - start);

    return .{
        .healthy = healthy,
        .response_time_ms = elapsed,
    };
}

fn doCheck(allocator: std.mem.Allocator, host: []const u8, port: u16, path: []const u8) bool {
    const stream = std.net.tcpConnectToHost(allocator, host, port) catch return false;
    defer stream.close();

    var req_buf: [256]u8 = undefined;
    const req = std.fmt.bufPrint(&req_buf, "GET {s} HTTP/1.1\r\nHost: {s}\r\nConnection: close\r\n\r\n", .{ path, host }) catch return false;
    stream.writeAll(req) catch return false;

    var resp_buf: [1024]u8 = undefined;
    const n = stream.read(&resp_buf) catch return false;
    if (n < 12) return false;

    const status = std.fmt.parseInt(u16, resp_buf[9..12], 10) catch return false;
    return status == 200;
}
