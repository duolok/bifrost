const std = @import("std");

pub const Metrics = struct {
    memory_used_kb: u64,
    memory_total_kb: u64,
    cpu_usage_percent: f64,
    open_fds: u64,
};

const MemInfo = struct {
    total: u64,
    available: u64,
};

pub fn collect() !Metrics {
    const mem = try readMemory();
    const cpu = try readCpu();
    const fds = try countFds();
    return Metrics{
        .memory_used_kb = mem.total - mem.available,
        .memory_total_kb = mem.total,
        .cpu_usage_percent = cpu,
        .open_fds = fds,
    };
}

fn readMemory() !MemInfo {
    const file = try std.fs.openFileAbsolute("/proc/meminfo", .{});
    defer file.close();

    var buf_reader = std.io.bufferedReader(file.reader());
    const reader = buf_reader.reader();

    var total: u64 = 0;
    var available: u64 = 0;
    var found: u8 = 0;

    var line_buf: [256]u8 = undefined;
    while (try reader.readUntilDelimiterOrEof(&line_buf, '\n')) |line| {
        if (std.mem.startsWith(u8, line, "MemTotal:")) {
            total = parseProcValue(line);
            found += 1;
        } else if (std.mem.startsWith(u8, line, "MemAvailable:")) {
            available = parseProcValue(line);
            found += 1;
        }

        if (found == 2) break;
    }

    return MemInfo{ .total = total, .available = available };
}

fn readCpu() !f64 {
    const file = try std.fs.openFileAbsolute("/proc/stat", .{});
    defer file.close();

    var buf_reader = std.io.bufferedReader(file.reader());
    var reader = buf_reader.reader();

    var line_buf: [512]u8 = undefined;
    const line = (try reader.readUntilDelimiterOrEof(&line_buf, '\n')) orelse return 0.0;

    var it = std.mem.tokenizeScalar(u8, line, ' ');
    _ = it.next();

    var total: u64 = 0;
    var idle: u64 = 0;
    var i: u8 = 0;
    while (it.next()) |token| {
        const val = std.fmt.parseInt(u64, token, 10) catch continue;
        total += val;
        if (i == 3) idle = val;
        i += 1;
    }

    if (total == 0) return 0.0;
    const used: f64 = @floatFromInt(total - idle);
    const total_f: f64 = @floatFromInt(total);
    return (used / total_f) * 100.0;
}

fn countFds() !u64 {
    var dir = std.fs.openDirAbsolute("/proc/self/fd", .{ .iterate = true }) catch return 0;
    defer dir.close();

    var count: u64 = 0;
    var iter = dir.iterate();
    while (try iter.next()) |_| {
        count += 1;
    }

    return count;
}

fn parseProcValue(line: []const u8) u64 {
    var it = std.mem.tokenizeScalar(u8, line, ' ');
    _ = it.next();
    const val = it.next() orelse return 0;
    return std.fmt.parseInt(u64, val, 10) catch 0;
}
