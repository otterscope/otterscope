# DigitalOcean Community draft — Otterscope on a Droplet

*Ready to post at https://www.digitalocean.com/community/questions — voice matched to Kalin's prior guide. Paste as-is or edit.*

---

**Title:** Self-hosting observability for your AI agents on a $6 Droplet

I've been running a few small LLM agents lately, and the thing that kept
biting me wasn't the model — it was not being able to *see* what the agent
actually did. Which tool did it loop on five times? Where did that run
quietly cost me a dollar? Did last week's prompt tweak make things worse or
better? The logs never quite tell you.

The hosted tools for this want your prompts on their servers, and the
self-hostable ones (Langfuse and friends) wanted me to stand up Postgres +
ClickHouse + Redis + S3 — four moving parts to log a few thousand LLM calls
a day. That's a lot of Droplet for a side project.

So I ended up building a small open-source one, Otterscope, and self-hosting
it is genuinely a one-liner. Figured I'd write down how I run it on a
DigitalOcean Droplet, because "AI agent observability" sounds heavier than it
actually is here.

## What it does, quickly

You point your agent's OpenTelemetry exporter at Otterscope and you get an
agent-*run*-first view: every trace becomes a run, with its steps, tool
loops, tokens, and cost. Click a run and you see the actual messages in and
out of each model call. There's a cost table, assertions and LLM-as-judge
evals scored onto your real runs, a compare view for "this week vs last," and
webhook alerts when your error rate or spend crosses a line.

The important part for hosting: it's **one static Go binary and one SQLite
file.** No database to provision, no cache, no object store. That's the whole
reason it fits on the smallest Droplet.

## The Droplet

A **Basic $6/mo Droplet** (1 vCPU, 1 GB RAM) is plenty unless you're pushing
serious volume — SQLite handles this scale comfortably. Spin one up with
Ubuntu, add your SSH key, done.

There are two ways to run it. Docker is the fastest; the raw binary with
systemd is the leanest.

### Option A — Docker (fastest)

If you ticked the Docker Marketplace image when creating the Droplet, or just
`apt install docker.io`, it's one command:

```sh
docker run -d --restart unless-stopped \
  -p 8317:8317 -p 4318:4318 \
  -v otterscope:/data \
  ghcr.io/otterscope/otterscope
```

`8317` is the web UI, `4318` is the standard OTLP ingest port your agents send
to. The `otterscope` volume keeps your data across restarts.

### Option B — the binary + systemd (leanest)

No Docker, ~15 MB resident:

```sh
# grab the linux binary from the releases page, then:
sudo mv otterscope /usr/local/bin/
sudo useradd --system --home /var/lib/otterscope --create-home otterscope
```

Drop a unit at `/etc/systemd/system/otterscope.service`:

```ini
[Unit]
Description=Otterscope
After=network.target

[Service]
User=otterscope
# -listen/-otlp are set explicitly here because Otterscope binds to
# localhost by default (see the security note below)
ExecStart=/usr/local/bin/otterscope serve -db /var/lib/otterscope/otterscope.db -listen :8317 -otlp :4318
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

```sh
sudo systemctl enable --now otterscope
```

### One security note worth reading

By default Otterscope binds to `127.0.0.1` — nothing is exposed to the
internet until you decide to. On a Droplet you have two sane choices:

1. **Keep it private (my preference for a personal instance):** leave the
   default localhost bind, and reach the UI over an SSH tunnel from your
   laptop: `ssh -L 8317:localhost:8317 you@your-droplet`, then open
   `http://localhost:8317`. Nothing is public, ever.
2. **Expose it deliberately:** pass `-listen :8317 -otlp :4318` (as the unit
   above does), and put a DigitalOcean **Cloud Firewall** in front so only
   your app servers can reach `4318` and only you can reach `8317`. Otterscope
   is single-user and has no login yet, so don't leave `8317` open to the
   world.

## Point an agent at it

Whatever framework you use — Pydantic AI, the OpenAI Agents SDK, LangGraph,
the Vercel AI SDK — it's the standard OpenTelemetry variable:

```sh
export OTEL_EXPORTER_OTLP_ENDPOINT=http://YOUR_DROPLET_IP:4318
```

Run your agent, refresh the UI, and the run shows up in a couple of seconds.
No SDK to install, no code changes if you're already emitting OTel.

Want to see it populated before wiring a real agent? `otterscope sample`
seeds a batch of realistic demo runs.

## What it adds up to

- **$6/mo** Droplet, and honestly a $4 one would do for light use.
- **Zero** managed-database cost — it's a file on the Droplet's disk. Snapshot
  the Droplet or `scp` the `.db` file and that's your backup.
- Unlimited retention by default (it's your disk); there's a `-retention` flag
  if you'd rather prune.

That's the pitch: the observability stack for your agents is a single process
sitting next to them, costing about as much as a couple of coffees a month.

## Try it

Repo and docs are here: **https://github.com/otterscope/otterscope**
(Apache-2.0). There are per-framework setup guides under `docs/frameworks/`,
and the Docker image is `ghcr.io/otterscope/otterscope`.

If you self-host it on a Droplet and hit a rough edge — especially a framework
whose traces don't map cleanly yet — open an issue. Raw payloads are retained,
so fixes apply retroactively to data you've already collected.
