<script>
	import { onMount } from 'svelte';

	let token = $state('');
	let tokenInput = $state('');
	let connected = $state(false);
	let game = $state({ map: '', game_state: '', score_ct: '', score_t: '', players: [] });
	let lines = $state([]);
	let cmd = $state('');
	let consoleLog = $state([]);
	let autoscroll = $state(true);
	let logBox = $state(null);
	let consoleBox = $state(null);

	let es = null;

	function connect() {
		if (!token) return;
		localStorage.setItem('cs2watch_token', token);
		es?.close();
		es = new EventSource(`/events?token=${encodeURIComponent(token)}`);
		es.onopen = () => (connected = true);
		es.onerror = () => (connected = false); // EventSource retries on its own
		es.onmessage = (e) => {
			const ev = JSON.parse(e.data);
			if (ev.type === 'snapshot') {
				game = ev.data.state ?? game;
				lines = ev.data.lines ?? [];
			} else if (ev.type === 'lines') {
				lines.push(...ev.data);
				if (lines.length > 2000) lines = lines.slice(-2000);
			} else if (ev.type === 'state') {
				ev.data.players ??= game.players;
				game = ev.data;
			} else if (ev.type === 'players') {
				game.players = ev.data ?? [];
			}
		};
	}

	function saveToken(e) {
		e.preventDefault();
		token = tokenInput.trim();
		connect();
	}

	async function send(e) {
		e.preventDefault();
		const command = cmd.trim();
		if (!command) return;
		cmd = '';
		try {
			const r = await fetch('/api/rcon', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
				body: JSON.stringify({ command })
			});
			const j = await r.json();
			consoleLog.push({ command, output: j.ok ? j.output : `error: ${j.error ?? r.status}` });
		} catch (err) {
			consoleLog.push({ command, output: `error: ${err.message}` });
		}
	}

	function lineClass(line) {
		if (line.includes(' killed ')) return 'kill';
		if (line.includes(' say ') || line.includes(' say_team ')) return 'chat';
		if (line.includes('connected') || line.includes('entered the game') || line.includes('disconnected'))
			return 'conn';
		if (line.includes('rcon from')) return 'rcon';
		return '';
	}

	function fmtDuration(s) {
		const m = Math.floor(s / 60);
		return m >= 60 ? `${Math.floor(m / 60)}h${String(m % 60).padStart(2, '0')}` : `${m}m`;
	}

	$effect(() => {
		void lines.length;
		if (autoscroll && logBox) logBox.scrollTop = logBox.scrollHeight;
	});

	$effect(() => {
		void consoleLog.length;
		if (consoleBox) consoleBox.scrollTop = consoleBox.scrollHeight;
	});

	onMount(() => {
		token = localStorage.getItem('cs2watch_token') ?? '';
		if (token) connect();
		return () => es?.close();
	});
</script>

<svelte:head><title>cs2-watch</title></svelte:head>

<div class="app">
	<header>
		<span class="brand">cs2-watch</span>
		<span class="dot" class:on={connected}></span>
		{#if connected || game.map}
			<span class="map">{game.map || '—'}</span>
			<span class="phase">{game.game_state || '—'}</span>
			<span class="score">CT {game.score_ct || 0} : {game.score_t || 0} T</span>
			<span class="count">{(game.players ?? []).length} players</span>
		{/if}
		{#if !connected}
			<form class="tokenform" onsubmit={saveToken}>
				<input type="password" placeholder="auth token" bind:value={tokenInput} />
				<button>connect</button>
			</form>
		{/if}
		<label class="scroll"><input type="checkbox" bind:checked={autoscroll} /> autoscroll</label>
	</header>

	<main>
		<aside>
			<table>
				<thead><tr><th>player</th><th>score</th><th>time</th></tr></thead>
				<tbody>
					{#each game.players ?? [] as p (p.name)}
						<tr><td>{p.name}</td><td>{p.score}</td><td>{fmtDuration(p.duration_s)}</td></tr>
					{:else}
						<tr><td colspan="3" class="empty">no players</td></tr>
					{/each}
				</tbody>
			</table>
		</aside>

		<section class="log" bind:this={logBox}>
			{#each lines as line}
				<div class="line {lineClass(line)}">{line}</div>
			{/each}
		</section>
	</main>

	<footer>
		<div class="console" bind:this={consoleBox}>
			{#each consoleLog as entry}
				<div class="cmd">&gt; {entry.command}</div>
				<pre>{entry.output}</pre>
			{/each}
		</div>
		<form onsubmit={send}>
			<input placeholder="rcon command… (status, mp_restartgame 1, …)" bind:value={cmd} />
			<button>send</button>
		</form>
	</footer>
</div>

<style>
	:global(html, body) {
		margin: 0;
		height: 100%;
		background: #101418;
		color: #cdd6dd;
		font-family: 'Cascadia Mono', Consolas, monospace;
		font-size: 13px;
	}
	.app {
		display: grid;
		grid-template-rows: auto 1fr auto;
		height: 100vh;
	}
	header {
		display: flex;
		align-items: center;
		gap: 1rem;
		padding: 0.5rem 0.75rem;
		background: #161c22;
		border-bottom: 1px solid #232b33;
	}
	.brand { font-weight: bold; color: #e8a33d; }
	.dot { width: 9px; height: 9px; border-radius: 50%; background: #b33; }
	.dot.on { background: #3b3; }
	.score { color: #7db3d9; }
	.count, .phase { color: #8a949c; }
	.tokenform, footer form { display: flex; gap: 0.4rem; flex: 1; }
	.scroll { margin-left: auto; color: #8a949c; user-select: none; }
	main {
		display: grid;
		grid-template-columns: 280px 1fr;
		min-height: 0;
	}
	aside { border-right: 1px solid #232b33; overflow-y: auto; }
	table { width: 100%; border-collapse: collapse; }
	th, td { text-align: left; padding: 0.25rem 0.5rem; border-bottom: 1px solid #1a2128; }
	th { color: #8a949c; position: sticky; top: 0; background: #101418; }
	td:nth-child(2), th:nth-child(2) { text-align: right; }
	.empty { color: #5a646c; }
	.log { overflow-y: auto; padding: 0.4rem 0.6rem; }
	.line { white-space: pre-wrap; word-break: break-all; line-height: 1.45; }
	.line.kill { color: #e06c60; }
	.line.chat { color: #98c379; }
	.line.conn { color: #61afef; }
	.line.rcon { color: #c678dd; }
	footer { border-top: 1px solid #232b33; background: #14191f; }
	.console { max-height: 180px; overflow-y: auto; padding: 0.3rem 0.6rem; }
	.console .cmd { color: #e8a33d; }
	.console pre { margin: 0 0 0.4rem 0; white-space: pre-wrap; color: #9aa5ad; }
	footer form { padding: 0.4rem 0.6rem; }
	input {
		flex: 1;
		background: #0c1014;
		border: 1px solid #2a333c;
		color: #cdd6dd;
		padding: 0.35rem 0.5rem;
		font: inherit;
	}
	button {
		background: #23415a;
		border: 1px solid #33587a;
		color: #cdd6dd;
		padding: 0.35rem 0.8rem;
		font: inherit;
		cursor: pointer;
	}
	button:hover { background: #2b4f6e; }
</style>
