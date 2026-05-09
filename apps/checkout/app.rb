require "json"
require "net/http"
require "sinatra/base"
require "opentelemetry/sdk"
require "opentelemetry/exporter/otlp"
require "opentelemetry/instrumentation/rack"
require "opentelemetry/instrumentation/net/http"

OpenTelemetry::SDK.configure do |c|
  c.service_name = "checkout"
  c.resource = OpenTelemetry::SDK::Resources::Resource.create(
    "service.name" => "checkout",
    "service.namespace" => "ecommerce",
    "deployment.environment.name" => "kind-lab"
  )
  c.use "OpenTelemetry::Instrumentation::Rack"
  c.use "OpenTelemetry::Instrumentation::Net::HTTP"
end

class CheckoutService < Sinatra::Base
  set :bind, "0.0.0.0"
  set :port, 8080
  set :host_authorization, {
    permitted_hosts: [
      "localhost",
      "127.0.0.1",
      "checkout",
      "checkout.ecommerce",
      "checkout.ecommerce.svc",
      "checkout.ecommerce.svc.cluster.local"
    ]
  }

  CATALOG_URL = ENV.fetch("CATALOG_URL", "http://catalog:8080")
  INVENTORY_URL = ENV.fetch("INVENTORY_URL", "http://inventory:8080")

  get "/healthz" do
    content_type :json
    { status: "ok" }.to_json
  end

  get "/checkout" do
    content_type :json
    product_id = params.fetch("product_id", "sku-astro-mug")
    force_error = params["force_error"] == "true"
    slow = params["slow"] == "true"

    tracer.in_span("checkout.orchestrate") do |span|
      span.set_attribute("product.id", product_id)
      product = get_json("#{CATALOG_URL}/products/#{product_id}")
      inventory_url = "#{INVENTORY_URL}/inventory/#{product_id}"
      inventory_url += "?slow=true" if slow
      stock = get_json(inventory_url)

      if force_error
        halt_with_error(span, 402, "pagamento recusado em cenário controlado", product_id)
      end

      unless stock.fetch("available")
        halt_with_error(span, 409, "produto sem estoque", product_id)
      end

      log_event("checkout-ok", "pedido aprovado", product_id: product_id)
      status 200
      { status: "approved", product: product, inventory: stock }.to_json
    end
  rescue StandardError => e
    current_span.record_exception(e)
    current_span.status = OpenTelemetry::Trace::Status.error(e.message)
    log_event("checkout-exception", e.message, product_id: product_id)
    status 502
    { status: "failed", error: e.message }.to_json
  end

  def get_json(url)
    uri = URI(url)
    request = Net::HTTP::Get.new(uri)
    OpenTelemetry.propagation.inject(request)
    response = Net::HTTP.start(uri.hostname, uri.port) { |http| http.request(request) }
    raise "HTTP #{response.code} from #{url}: #{response.body}" unless response.is_a?(Net::HTTPSuccess)
    JSON.parse(response.body)
  end

  def halt_with_error(span, code, message, product_id)
    span.status = OpenTelemetry::Trace::Status.error(message)
    log_event("checkout-error", message, product_id: product_id, status: code)
    status code
    body({ status: "failed", error: message, product_id: product_id }.to_json)
    throw :halt
  end

  def log_event(event, message, fields = {})
    context = current_span.context
    puts(
      {
        "service.name" => "checkout",
        event: event,
        message: message,
        trace_id: context.hex_trace_id,
        span_id: context.hex_span_id
      }.merge(fields).to_json
    )
  end

  def current_span
    OpenTelemetry::Trace.current_span
  end

  def tracer
    OpenTelemetry.tracer_provider.tracer("checkout")
  end

  run! if app_file == $PROGRAM_NAME
end
