# Request Decompressor Module for Caddy

This Caddy module provides middleware for automatically decompressing incoming HTTP requests that use various compression methods (gzip, bzip2, zstd).

## Features

- Supports multiple compression formats:
  - gzip
  - bzip2 (bz2)
  - zstd
- Automatically detects and decompresses requests based on Content-Encoding header
- Returns 400 Bad Request for malformed compressed data
- Includes metrics for monitoring decompression operations
- Preserves original request content while removing Content-Encoding header after decompression

## Installation

To build Caddy with this module:

```bash
xcaddy build --with github.com/calebcall/request-decompressor
```

## Usage

### Caddyfile

Add the middleware to your Caddyfile:

```caddyfile
{
    order request_decompress before reverse_proxy
}

localhost {
    request_decompress
    reverse_proxy backend:8080
}
```

### Example Request

```bash
# Send a gzipped JSON request
echo '{ "jsonrpc":"2.0", "method":"eth_chainId", "params":[], "id":1 }' | \
    gzip | \
    curl -i --request POST \
    --url http://localhost \
    --header 'Content-Type: application/json' \
    --header 'Content-Encoding: gzip' \
    --compressed \
    --data-binary @-
```

## Metrics

The module tracks the following metrics:

- Total requests processed
- Successful decompression operations
- Failed decompression operations
- Decompression timing
- Request counts by compression type

## License

Apache 2.0
