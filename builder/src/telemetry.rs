use anyhow::Result;
use opentelemetry::global;
use opentelemetry::trace::TracerProvider as _;
use opentelemetry::KeyValue;
use opentelemetry_otlp::WithExportConfig;
use opentelemetry_sdk::{trace as sdktrace, Resource};
use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt, EnvFilter};

pub fn init() -> Result<()> {
    let endpoint = std::env::var("OTEL_EXPORTER_OTLP_ENDPOINT").ok();

    let env_filter = EnvFilter::from_default_env();
    let fmt_layer = tracing_subscriber::fmt::layer().json();

    if let Some(endpoint) = endpoint {
        let exporter = opentelemetry_otlp::SpanExporter::builder()
            .with_tonic()
            .with_endpoint(&endpoint)
            .build()?;

        let provider = sdktrace::TracerProvider::builder()
            .with_batch_exporter(exporter, opentelemetry_sdk::runtime::Tokio)
            .with_resource(Resource::new(vec![KeyValue::new(
                "service.name",
                "bifrost-builder",
            )]))
            .build();

        global::set_tracer_provider(provider.clone());

        let otel_layer = tracing_opentelemetry::layer()
            .with_tracer(provider.tracer("bifrost-builder"));

        tracing_subscriber::registry()
            .with(env_filter)
            .with(fmt_layer)
            .with(otel_layer)
            .init();

        tracing::info!(%endpoint, "OpenTelemetry tracing enabled");
    } else {
        tracing_subscriber::registry()
            .with(env_filter)
            .with(fmt_layer)
            .init();

        tracing::info!("OTEL_EXPORTER_OTLP_ENDPOINT not set, tracing disabled");
    }

    Ok(())
}

pub fn shutdown() {
    global::shutdown_tracer_provider();
}
