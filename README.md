# Cloudflared Proxy

![logo](./img/logo.png#gh-light-mode-only)
![logo](./img/logo_dark.png#gh-dark-mode-only)

A flexible reverse proxy for Cloudflare Access applications.

This tool allows you to proxy multiple Cloudflare Access protected applications to your local machine, with easy configuration via command-line flags, a configuration file, or environment variables.

## Features

- **Multiple Endpoints**: Proxy multiple applications simultaneously.
- **Flexible Configuration**: Use command-line flags, a configuration file (YAML, JSON, etc.), or environment variables.
- **TLS Configuration**: Option to skip TLS verification for non trusted certificates.

## Installation

The binary can be downloaded from the [GitHub Releases](https://github.com/sbldevnet/cloudflared-proxy/releases) page.

Alternatively, you can build from source:

```bash
go build -o cfproxy .
```

## Usage

The primary command is `run`, which starts the reverse proxies.

```bash
./cfproxy run [flags]
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
./cfproxy run -e example.com

# Proxy example.com to localhost:9000
./cfproxy run -e 9000:example.com

# Proxy example.com:8443 to localhost:8888
./cfproxy run -e example.com:8443

# Proxy example.com:8443 to localhost:9000
./cfproxy run -e 9000:example.com:8443

# Proxy multiple endpoints
./cfproxy run -e example1.com,9001:example2.com
  # or
./cfproxy run -e example1.com -e 9001:example2.com

# Skip TLS verification
./cfproxy run -e example.com --skip-tls
```

### Configuration File

For a more persistent setup, you can use a configuration file. By default, `cfproxy` looks for a `config` file in `$HOME/.config/cloudflared-proxy/`. You can specify a different file with the `--config` or `-c` flag.

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
./cfproxy run
```

Or with a custom config file path:
```bash
./cfproxy run -c /path/to/your/config.yaml
```

### Configuration Precedence

The tool can be configured using a configuration file or command-line flags. Please note that for defining proxy endpoints, **these methods are mutually exclusive**.

Configuration is loaded from the following sources:

1.  **Configuration File**:
    - By default, the tool looks for a `config.yaml` file in `$HOME/.config/cloudflared-proxy/`.
    - A different configuration file can be specified using the `--config` or `-c` flag. If a file is specified with this flag and it is not found, the program will exit with an error.
2.  **Command-Line Flags**:
    - Endpoints can be specified directly using the `--endpoints` or `-e` flag.
    - If endpoints are provided via flags, any configuration file will be ignored for endpoint definitions.

---

For more details on Cloudflare Tunnels, see the [official documentation](https://developers.cloudflare.com/cloudflare-one/tutorials/cli/).
