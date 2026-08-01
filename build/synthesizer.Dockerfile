FROM node:22-bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends git jq util-linux ca-certificates && rm -rf /var/lib/apt/lists/*
RUN npm install -g @anthropic-ai/claude-code

RUN mkdir -p /data && chown -R node:node /data

COPY build/synthesizer/run.sh build/synthesizer/synth.sh build/synthesizer/lib.sh build/synthesizer/intake.sh build/synthesizer/intake-run.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/run.sh /usr/local/bin/synth.sh /usr/local/bin/lib.sh /usr/local/bin/intake.sh /usr/local/bin/intake-run.sh

ENV HOME=/home/node

USER node

ENTRYPOINT ["run.sh"]
