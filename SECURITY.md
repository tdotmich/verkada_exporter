# Security

## Report a vulnerability

Do not open a public issue for a security problem.
Use GitHub private vulnerability reporting: **Security** tab > **Report a vulnerability**.

## How the exporter handles secrets

- `VERKADA_API_KEY` comes only from an environment variable.
- The API key goes only to `POST /token`. All other calls use the short-lived token.
- The exporter does not follow HTTP redirects, so a redirect cannot send the API key to another host.
- The exporter never logs the API key or the token.
- The exporter does not export device IP addresses or map locations.
- `/metrics` contains device names, sites, serial numbers and sensor readings. Use `-web.config.file`
  for TLS and basic auth, or keep the port on a private network.
