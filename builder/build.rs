fn main() -> Result<(), Box<dyn std::error::Error>> {
    // In Docker, proto is copied to ./proto/events.proto
    // Locally, it's at ../proto/events.proto
    let proto_path = if std::path::Path::new("proto/events.proto").exists() {
        "proto/events.proto"
    } else {
        "../proto/events.proto"
    };
    tonic_build::compile_protos(proto_path)?;
    Ok(())
}
