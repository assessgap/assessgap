"""Daily traffic summary from Caddy's JSON access log. Runs on the web server
(see ops/stats.sh). It prints counts only, never IPs or PINs.

usage: python3 stats.py [days] [logdir]
"""
import collections, datetime, glob, gzip, json, os, re, sys
from urllib.parse import urlsplit, parse_qs
from zoneinfo import ZoneInfo

days = int(sys.argv[1]) if len(sys.argv) > 1 else 7
logdir = sys.argv[2] if len(sys.argv) > 2 else "/var/log/caddy"
tz = ZoneInfo("America/Chicago")
since = datetime.datetime.now(tz).date() - datetime.timedelta(days=days - 1)

BOT = re.compile(r"bot|crawl|spider|slurp|curl|wget|python|go-http|httpx|scan|preview|monitor|uptime|headless", re.I)
PAGES = {"/", "/how-it-works", "/terms", "/privacy"}
REPORT = re.compile(r"^/check/[0-9-]+$")
PDF = re.compile(r"^/check/[0-9-]+/report\.pdf$")

day = collections.defaultdict(lambda: collections.Counter())
visitors = collections.defaultdict(set)
referrers, sources, bots = collections.Counter(), collections.Counter(), collections.Counter()

def lines():
    for path in sorted(glob.glob(os.path.join(logdir, "access*.log*"))):
        op = gzip.open if path.endswith(".gz") else open
        with op(path, "rt", errors="replace") as f:
            yield from f

for line in lines():
    try:
        e = json.loads(line)
    except ValueError:
        continue
    req = e.get("request", {})
    if req.get("host") != "assessgap.com":
        continue
    d = datetime.datetime.fromtimestamp(e["ts"], tz).date()
    if d < since:
        continue
    h = req.get("headers", {})
    ua = (h.get("User-Agent") or [""])[0]
    u = urlsplit(req.get("uri", ""))
    path, status, method = u.path, e.get("status", 0), req.get("method")
    c = day[d]
    if status >= 500:
        c["5xx"] += 1
        if status == 503:
            c["busy (503)"] += 1
    # Real browsers send Sec-Fetch-Mode; most scanners faking a browser UA don't.
    if BOT.search(ua) or not ua or "Sec-Fetch-Mode" not in h:
        if method == "GET" and status == 200 and not path.startswith("/static/"):
            bots[d] += 1
        continue
    htmx = "Hx-Request" in h or "HX-Request" in h
    if method == "GET" and status == 200 and not htmx and (path in PAGES or REPORT.match(path)):
        c["page views"] += 1
        visitors[d].add(req.get("client_ip") or req.get("remote_ip"))
        ref = (h.get("Referer") or [""])[0]
        host = urlsplit(ref).hostname or ""
        if host and not host.endswith("assessgap.com"):
            referrers[host.removeprefix("www.")] += 1
        for src in parse_qs(u.query).get("src", []):
            sources[src[:40]] += 1
    if method == "GET" and path == "/search" and not htmx:
        c["searches"] += 1
    if method == "GET" and status == 200 and REPORT.match(path):
        c["reports viewed"] += 1
    if method == "GET" and status == 200 and PDF.match(path):
        c["PDFs downloaded"] += 1
    if method == "POST" and path == "/notify" and status < 400:
        c["email signups"] += 1

cols = ["visitors", "page views", "searches", "reports viewed", "PDFs downloaded", "email signups", "5xx", "busy (503)", "bot hits"]
print(f"{'day':<11}" + "".join(f"{k:>16}" for k in cols))
tot = collections.Counter()
for i in range(days):
    d = since + datetime.timedelta(days=i)
    row = dict(day[d]); row["visitors"] = len(visitors[d]); row["bot hits"] = bots[d]
    tot.update(row)
    print(f"{d.isoformat():<11}" + "".join(f"{row.get(k, 0):>16}" for k in cols))
print(f"{'total':<11}" + "".join(f"{tot.get(k, 0):>16}" for k in cols) + "   (visitors: sum of daily uniques)")
for title, counter in (("Referrers", referrers), ("?src= tags", sources)):
    if counter:
        print(f"\n{title}:")
        for k, n in counter.most_common(10):
            print(f"  {n:>6}  {k}")
