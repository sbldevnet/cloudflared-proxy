# Cloudflared Proxy

![logo](./img/logo.png#gh-light-mode-only)
![logo](./img/logo_dark.png#gh-dark-mode-only)

A flexible reverse proxy for Cloudflare Access applications.

This tool allows you to proxy multiple Cloudflare Access protected applications to your local machine, with easy configuration via command-line flags or a configuration file.

## Features

- **Multiple Endpoints**: Proxy multiple applications simultaneously.
- **Flexible Configuration**: Use command-line flags or a configuration file (YAML, JSON, etc.).
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

---

For more details on Cloudflare Tunnels, see the [official documentation](https://developers.cloudflare.com/cloudflare-one/tutorials/cli/).
