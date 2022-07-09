# project tucson

*It's a place in near Phoenix...*

## Usage

Tucson is a very basic OIDC capable gateway.  It was built out of a need to add auth some backend systems
with heterogenous programming languages and mixed capabilities.  It was also built out of curiosity and an interest
in learning more about how a go proxy might work and how OIDC might work through a proxy.  Trade-offs were made in
it's implementation and I make no guarentees about it's production suitability.

Tuscon is configured with [origins](###-origins) and [matchers](###-matchers).  

### Origins

Origins are the basic building block for proxy backends.  Origns have a name key and a configuration.  The simplest way to
configure origins is through a config file, although it should be possible to configure through the environment as
well.  Origins support the following parameters:

| Parameter     | Type  | Description |
| ------------- | ----- | ------------|
| `url`         | string            | the backend url to proxy to |
| `set_headers` | map[string]string | override headers in the request to the backend |
| `add_header`  | map[string]string | append headers in the request to the backend |
| `insecure`    | bool              | ignore tls errors in backend requests |
| `oidc`        | bool              | enable/disable oidc for connections to the origin |

ex.

```json
"origins": {
  "example": {
    "url": "https:/www.example.com",
    "set_headers": {
      "Host": "www.example.com"
    },
    "insecure": true,
    "oidc": true
  },
  "google": {
    "url": "https://www.google.com",
    "set_headers": {
      "Host": "www.google.com"
    },
    "insecure": false,
    "oidc": false
  }
},
```

### Matchers

Matchers link a url to an origin.  The matchers are processed in order with the first match winning.  Path patterns are passed
directly as [chi router patterns]().  Matchers support the following parameters:

| Parameter | Type   | Description |
| --------- | ------ | ------------|
| `path`    | string | the chi router pattern for matching requests |
| `origin`  | string | the name of origin to select for the pattern |

ex.

```json
  "matchers": [
    {
      "path": "/foo/*",
      "origin": "google"
    },
    {
      "path": "/bar",
      "origin": "example"
    }
  ]
```

### Default Origins

The configuration also accepts a `default_origin` for anything that falls through.

### Limitations

Tucson does not rewrite links and URLs in the payload from backend systems, so they need to be proxy aware.

## License

```
Copyright 2022 E Camden Fisher

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
```