# Cloudflared Proxy

![logo](./img/logo.png#gh-light-mode-only)
![logo](./img/logo_dark.png#gh-dark-mode-only)

A flexible reverse proxy for Cloudflare Access applications.

This tool allows you to proxy multiple Cloudflare Access protected applications to your local machine, with easy configuration via command-line flags or a configuration file.

## Features

- **Multiple Endpoints**: Proxy multiple applications simultaneously.
- **Flexible Configuration**: Use command-line flags or a configuration file (YAML, JSON, etc.).
- **Local by Default**: Proxies listen on `127.0.0.1` unless you choose another address.
- **TLS Configuration**: Option to skip TLS verification for non trusted certificates.

## Installation

The binary can be downloaded from the [GitHub Releases](https://github.com/sbldevnet/cloudflared-proxy/releases) page.

Alternatively, you can build from source:
```bash
go build -o cloudflared-proxy .
```

## Usage

The primary command is `run`, which starts the reverse proxies.
```bash
./cloudflared-proxy run [flags]
```

### Command-Line Flags

You can specify endpoints directly on the command line.

**Endpoint Format:** `[LOCAL_PORT:]HOSTNAME[:DEST_PORT]`

- `LOCAL_PORT`: (Optional) The port on your local machine (default: `8888`).
- `HOSTNAME`: (Required) The destination hostname.
- `DEST_PORT`: (Optional) The destination port (default: `443`).

**Examples:**
```bash
# Proxy example.com to localhost:8888
./cloudflared-proxy run -e example.com

# Proxy example.com to localhost:9000
./cloudflared-proxy run -e 9000:example.com

# Proxy example.com:8443 to localhost:8888
./cloudflared-proxy run -e example.com:8443

# Proxy example.com:8443 to localhost:9000
./cloudflared-proxy run -e 9000:example.com:8443

# Proxy multiple endpoints
./cloudflared-proxy run -e example1.com,9001:example2.com
  # or
./cloudflared-proxy run -e example1.com -e 9001:example2.com

# Skip TLS verification
./cloudflared-proxy run -e example.com --skip-tls

# Listen on another address (see "Exposing the proxy")
./cloudflared-proxy run -e example.com --listen 0.0.0.0
```

### Configuration File

For a more persistent setup, you can use a configuration file. By default, `cloudflared-proxy` looks for a `config` file in `$HOME/.config/cloudflared-proxy/`. You can specify a different file with the `--config` or `-c` flag.

**Example `config.yaml`:**
```yaml
proxies:
  - hostname: "app1.your-domain.com"
    localPort: 8080
  - hostname: "app2.your-domain.com"
    localPort: 8081
    destinationPort: 8443
  - hostname: "app3.your-domain.com"
    skipTLS: true
```

To listen on another address, set `listen` at the top level (default for every proxy) and/or per proxy:
```yaml
listen: 0.0.0.0        # default for all proxies below
proxies:
  - hostname: "app1.your-domain.com"
  - hostname: "app2.your-domain.com"
    listen: 127.0.0.1  # this one stays local
```

With a configuration file, you can start the proxies with a simple command:
```bash
./cloudflared-proxy run
```

Or with a custom config file path:
```bash
./cloudflared-proxy run -c /path/to/your/config.yaml
```

### Configuration Precedence

**Important**: Command-line flags and explicit configuration files are **mutually exclusive** for defining proxy endpoints.

Configuration priority:

1. **Command-Line Flags** (`--endpoints`):
   - When provided, all endpoint configuration comes from flags
   - Any configuration file (default or explicit) is ignored
   - Example: `./cloudflared-proxy run -e example.com`

2. **Explicit Configuration File** (`--config`):
   - When provided, all endpoint configuration comes from this file
   - Cannot be combined with `--endpoints` flag
   - If the specified file is not found, the program will exit with an error
   - Example: `./cloudflared-proxy run -c /path/to/config.yaml`

3. **Default Configuration File**:
   - If neither flags nor explicit config are provided, the tool looks for `config.yaml` in `$HOME/.config/cloudflared-proxy/`
   - If not found, the program will display help information
   - Example: `./cloudflared-proxy run`

### Skipping TLS verification

`--skip-tls` skips certificate verification for every proxy, in both `--endpoints` and config-file mode, and takes precedence over the per-proxy `skipTLS` key. A warning is logged for each proxy that skips verification.

The log level and format can be set with `--log-level` and `--log-format` or with the `logLevel` and `logFormat` config keys. The `LOG_LEVEL` and `LOG_FORMAT` environment variables still work. Precedence is flag, environment variable, config file, then the defaults (`info`, `text`).

### Exposing the proxy

By default every proxy listens on `127.0.0.1`. The proxy adds your Cloudflare Access token to every request it forwards, so only expose it on networks you trust.

To listen on another address, use `--listen ADDR` or the `listen` config key (an IP address, not a hostname). The flag overrides the config file, and a per-proxy `listen` overrides the top-level one.

If a client cannot connect through `localhost` (it only tries `::1`), use `127.0.0.1` instead.

## Use as a library

The `proxy` package can be embedded in other Go programs:

```go
import (
	"github.com/sbldevnet/cloudflared-proxy/config"
	"github.com/sbldevnet/cloudflared-proxy/proxy"
)

err := proxy.New().Run(ctx, []config.ProxyConfig{
	{Hostname: "example.com", DestinationPort: 443, LocalPort: 8888},
})
```

`Run` blocks until `ctx` is cancelled. Set `ProxyConfig.Listen` to an IP address literal to change the listen address; an empty value means `127.0.0.1`.

---

For more details on Cloudflare Tunnels, see the [official documentation](https://developers.cloudflare.com/cloudflare-one/tutorials/cli/).

