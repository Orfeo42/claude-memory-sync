FROM node:22-bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends git jq ca-certificates && rm -rf /var/lib/apt/lists/*
RUN npm install -g @anthropic-ai/claude-code

RUN mkdir -p /data && chown -R node:node /data

COPY build/synthesizer/run.sh build/synthesizer/synth.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/run.sh /usr/local/bin/synth.sh

ENV HOME=/home/node

USER node

ENTRYPOINT ["run.sh"]
