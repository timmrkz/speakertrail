---
name: docker
description: How to change and test the Docker setup Tim runs on his Mac, from a cloud session. Use for any change to the Makefile's Docker targets, compose.yaml, the Dockerfile, or a new tool the project needs.
---

# Changing the Docker setup

Tim runs everything through `make`, which drives `docker compose` on his
Mac. A cloud session runs the same targets directly. So a change to the
Docker side is easy to get wrong without noticing, because the session never
takes that path unless asked to.

## Rules

- A new tool goes into the `dev` stage of the `Dockerfile`. The production
  stages only get what the deployed app needs.
- A new target in the `Makefile` gets both branches: the Docker one, which
  usually runs the target in the toolbox, and the direct one.
- Ports are published on `127.0.0.1` only.
- Whatever the containers keep goes into a named volume, never into a
  folder on the Mac other than the repository.
- The Mac is an arm64 machine. Every base image has to exist for arm64.

## Testing in a cloud session

Docker works here, but containers reach nothing on the internet, and the
session's proxy does not allow Debian's package servers. So:

1. Start Docker if `docker info` fails: `dockerd >/dev/null 2>&1 &`.
2. Make a scratch copy of the `Dockerfile` in the scratchpad that, after
   every `FROM`, copies `/root/.ccr/ca-bundle.crt` in and sets
   `HTTPS_PROXY`, `HTTP_PROXY`, `SSL_CERT_FILE` and `NODE_EXTRA_CA_CERTS`,
   and leaves out the `apt-get` steps.
3. A compose override in the scratchpad points the builds at it, with
   `network: host` and `additional_contexts: {ccr: /root/.ccr}`, and gives
   `dev` the environment `GOPROXY=off` with the proxy variables empty.
4. Fill the toolbox's volumes from the session:
   `speakertrail_gomod` from `$(go env GOMODCACHE)` and
   `speakertrail_node_modules` from `web/node_modules`.
5. Run with `COMPOSE_FILE=compose.yaml:OVERRIDE` and without
   `CLAUDE_CODE_REMOTE`, so make takes the Docker way.

The scratch copy has no Chromium in the toolbox, so the browser tests skip
there. CI's `toolbox` job builds the real `Dockerfile` and runs `make test`
in it, Chromium included. That job is the proof.

Afterwards `make stop`, and move no scratch file into the repository.
