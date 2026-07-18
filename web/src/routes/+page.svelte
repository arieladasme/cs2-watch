<script>
	import { onMount } from 'svelte';

	let token = $state('');
	let tokenInput = $state('');
	let connected = $state(false);
	let game = $state({ map: '', game_state: '', score_ct: '', score_t: '', players: [], roster: [] });
	let lines = $state([]);
	let chat = $state([]);
	let cmd = $state('');
	let consoleLog = $state([]);
	let autoscroll = $state(true);
	let tab = $state('log');
	let meta = $state({ quick_commands: [], maps: [] });
	let mapSel = $state('');
	let bansList = $state([]);
	let sayMsg = $state('');
	let banSteamId = $state('');
	let banReason = $state('');
	let logBox = $state(null);
	let chatBox = $state(null);
	let consoleBox = $state(null);

	let es = null;

	const roster = $derived(game.roster ?? []);
	const hasRoster = $derived(roster.length > 0);

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
				chat = ev.data.chat ?? [];
			} else if (ev.type === 'lines') {
				lines.push(...ev.data);
				if (lines.length > 2000) lines = lines.slice(-2000);
			} else if (ev.type === 'state') {
				ev.data.players ??= game.players;
				ev.data.roster ??= game.roster;
				game = ev.data;
			} else if (ev.type === 'players') {
				game.players = ev.data ?? [];
			} else if (ev.type === 'roster') {
				game.roster = ev.data ?? [];
			} else if (ev.type === 'chat') {
				chat.push(ev.data);
				if (chat.length > 200) chat = chat.slice(-200);
			}
		};
		fetchMeta();
	}

	async function fetchMeta() {
		try {
			const r = await fetch('/api/meta', { headers: { Authorization: `Bearer ${token}` } });
			if (r.ok) meta = await r.json();
		} catch {}
	}

	function saveToken(e) {
		e.preventDefault();
		token = tokenInput.trim();
		connect();
	}

	async function run(command) {
		if (!command) return;
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

	function send(e) {
		e.preventDefault();
		const command = cmd.trim();
		cmd = '';
		run(command);
	}

	function kick(p) {
		if (confirm(`Kick ${p.name}?`)) run(`kickid ${p.userid}`);
	}

	async function api(path, body) {
		const r = await fetch(path, {
			method: body ? 'POST' : 'GET',
			headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
			body: body ? JSON.stringify(body) : undefined
		});
		return r.ok ? r.json() : Promise.reject(new Error(r.status));
	}

	async function loadBans() {
		try {
			bansList = (await api('/api/bans')) ?? [];
		} catch {}
	}

	async function ban(p) {
		const reason = prompt(`Ban ${p.name} — razón:`);
		if (reason === null) return;
		try {
			await api('/api/bans', { steamid: p.steamid, name: p.name, reason });
			consoleLog.push({ command: `ban ${p.name}`, output: `banned ${p.steamid}` });
			loadBans();
		} catch (err) {
			consoleLog.push({ command: `ban ${p.name}`, output: `error: ${err.message}` });
		}
	}

	async function manualBan(e) {
		e.preventDefault();
		const steamid = banSteamId.trim();
		if (!steamid) return;
		try {
			await api('/api/bans', { steamid, reason: banReason.trim() });
			banSteamId = '';
			banReason = '';
			loadBans();
		} catch (err) {
			alert(`error: ${err.message}`);
		}
	}

	async function unban(steamid) {
		if (!confirm(`Unban ${steamid}?`)) return;
		await api('/api/unban', { steamid }).catch(() => {});
		loadBans();
	}

	function sendSay(e) {
		e.preventDefault();
		const msg = sayMsg.trim();
		sayMsg = '';
		if (msg) run(`say ${msg}`);
	}

	function copySid(sid) {
		navigator.clipboard?.writeText(sid);
	}

	function openTab(t) {
		tab = t;
		if (t === 'bans') loadBans();
	}

	function changeMap() {
		if (mapSel) run(`changelevel ${mapSel}`);
	}

	function hsPct(p) {
		return p.frags > 0 ? Math.round((p.hs / p.frags) * 100) + '%' : '—';
	}

	function teamTag(t) {
		return t === 'TERRORIST' ? 'T' : t === 'CT' ? 'CT' : t === 'Spectator' ? 'SPEC' : '—';
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
		void tab;
		if (autoscroll && logBox) logBox.scrollTop = logBox.scrollHeight;
	});

	$effect(() => {
		void chat.length;
		void tab;
		if (chatBox) chatBox.scrollTop = chatBox.scrollHeight;
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
			<span class="score"><b class="ct">CT {game.score_ct || 0}</b> : <b class="t">{game.score_t || 0} T</b></span>
			<span class="count">{hasRoster ? roster.length : (game.players ?? []).length} players</span>
		{/if}
		{#if !connected}
			<form class="tokenform" onsubmit={saveToken}>
				<input type="password" placeholder="auth token" bind:value={tokenInput} />
				<button>connect</button>
			</form>
		{/if}
		<label class="scroll"><input type="checkbox" bind:checked={autoscroll} /> autoscroll</label>
		<span class="links">
			<a href="https://github.com/arieladasme/cs2-watch" target="_blank" rel="noopener">GitHub</a>
			<a href="https://ko-fi.com/arieladasme" target="_blank" rel="noopener" title="apoya el proyecto">☕</a>
		</span>
	</header>

	{#if connected}
		<div class="actions">
			{#each meta.quick_commands ?? [] as qc}
				<button onclick={() => run(qc.command)}>{qc.label}</button>
			{/each}
			{#if (meta.maps ?? []).length}
				<span class="sep"></span>
				<select bind:value={mapSel}>
					<option value="" disabled selected>map…</option>
					{#each meta.maps as m}<option value={m}>{m}</option>{/each}
				</select>
				<button onclick={changeMap} disabled={!mapSel}>changelevel</button>
			{/if}
		</div>
	{/if}

	<main>
		<aside>
			{#if hasRoster}
				<table>
					<thead><tr><th></th><th>player</th><th>K</th><th>D</th><th>ping</th><th></th></tr></thead>
					<tbody>
						{#each roster as p}
							<tr>
								<td class="team {p.team === 'TERRORIST' ? 't' : p.team === 'CT' ? 'ct' : ''}">{teamTag(p.team)}</td>
								<td class="name" title="A: {p.assists} · HS: {hsPct(p)}{p.addr ? ' · ' + p.addr.split(':')[0] : ''}">
									<div>{p.name}{p.bot ? ' 🤖' : ''}</div>
									{#if !p.bot && p.steamid}
										<button class="sid" title="click = copiar" onclick={() => copySid(p.steamid)}>{p.steamid}</button>
									{/if}
								</td>
								<td class="num">{p.frags}</td>
								<td class="num">{p.deaths}</td>
								<td class="num">{p.bot ? '—' : p.ping}</td>
								<td class="acts">
									<button class="kickbtn" title="kick" onclick={() => kick(p)}>✕</button>
									{#if !p.bot && p.steamid}
										<button class="banbtn" title="ban" onclick={() => ban(p)}>⛔</button>
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{:else}
				<table>
					<thead><tr><th>player</th><th>score</th><th>time</th></tr></thead>
					<tbody>
						{#each game.players ?? [] as p}
							<tr><td>{p.name || '(bot)'}</td><td class="num">{p.score}</td><td>{fmtDuration(p.duration_s)}</td></tr>
						{:else}
							<tr><td colspan="3" class="empty">no players</td></tr>
						{/each}
					</tbody>
				</table>
			{/if}
		</aside>

		<section class="right">
			<nav class="tabs">
				<button class:active={tab === 'log'} onclick={() => openTab('log')}>Log</button>
				<button class:active={tab === 'chat'} onclick={() => openTab('chat')}>Chat {chat.length ? `(${chat.length})` : ''}</button>
				<button class:active={tab === 'bans'} onclick={() => openTab('bans')}>Bans {bansList.length ? `(${bansList.length})` : ''}</button>
			</nav>
			{#if tab === 'log'}
				<div class="log" bind:this={logBox}>
					{#each lines as line}
						<div class="line {lineClass(line)}">{line}</div>
					{/each}
				</div>
			{:else if tab === 'chat'}
				<div class="log chatlog" bind:this={chatBox}>
					{#each chat as c}
						<div class="line">
							<span class={c.team === 'TERRORIST' ? 't' : c.team === 'CT' ? 'ct' : ''}>{c.name}</span
							>{c.team_only ? ' (team)' : ''}: <span class="msg">{c.msg}</span>
						</div>
					{:else}
						<div class="empty">no chat yet</div>
					{/each}
				</div>
				<form class="sayform" onsubmit={sendSay}>
					<input placeholder="decir algo en el server…" bind:value={sayMsg} />
					<button>say</button>
				</form>
			{:else}
				<div class="log banspane">
					<form class="banform" onsubmit={manualBan}>
						<input placeholder="steamid ([U:1:xxxx])" bind:value={banSteamId} />
						<input placeholder="razón" bind:value={banReason} />
						<button>ban offline</button>
					</form>
					<table>
						<thead><tr><th>name</th><th>steamid</th><th>ip</th><th>razón</th><th>fecha</th><th></th></tr></thead>
						<tbody>
							{#each bansList as b}
								<tr>
									<td>{b.name || '—'}</td>
									<td><button class="sid" onclick={() => copySid(b.steamid)}>{b.steamid}</button></td>
									<td>{b.ip ? b.ip.split(':')[0] : '—'}</td>
									<td>{b.reason || '—'}</td>
									<td>{new Date(b.created_at).toLocaleString()}</td>
									<td><button class="kickbtn" onclick={() => unban(b.steamid)}>unban</button></td>
								</tr>
							{:else}
								<tr><td colspan="6" class="empty">sin bans</td></tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
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
		grid-template-rows: auto auto 1fr auto;
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
	.ct { color: #6ea8dc; }
	.t { color: #d9a05b; }
	.count, .phase { color: #8a949c; }
	.tokenform, footer form { display: flex; gap: 0.4rem; flex: 1; }
	.scroll { margin-left: auto; color: #8a949c; user-select: none; }
	.links { display: flex; gap: 0.7rem; }
	.links a { color: #8a949c; text-decoration: none; }
	.links a:hover { color: #e8a33d; }
	.actions {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		padding: 0.35rem 0.75rem;
		background: #12171d;
		border-bottom: 1px solid #232b33;
		flex-wrap: wrap;
	}
	.actions .sep { width: 1px; height: 1.2rem; background: #2a333c; margin: 0 0.3rem; }
	main {
		display: grid;
		grid-template-columns: 400px 1fr;
		min-height: 0;
	}
	aside { border-right: 1px solid #232b33; overflow-y: auto; }
	table { width: 100%; border-collapse: collapse; }
	th, td { text-align: left; padding: 0.25rem 0.45rem; border-bottom: 1px solid #1a2128; }
	th { color: #8a949c; position: sticky; top: 0; background: #101418; }
	.num { text-align: right; }
	.name { max-width: 140px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.team { font-weight: bold; }
	.team.ct { color: #6ea8dc; }
	.team.t { color: #d9a05b; }
	.kickbtn, .banbtn {
		background: none;
		border: none;
		color: #5a646c;
		cursor: pointer;
		padding: 0 0.2rem;
	}
	.kickbtn:hover { color: #e06c60; }
	.banbtn:hover { color: #e8a33d; }
	.acts { white-space: nowrap; }
	.sid {
		background: none;
		border: none;
		color: #5a92b8;
		cursor: pointer;
		font-size: 11px;
		padding: 0;
		font-family: inherit;
	}
	.sid:hover { text-decoration: underline; }
	.sayform, .banform { display: flex; gap: 0.4rem; padding: 0.4rem 0.6rem; border-top: 1px solid #232b33; }
	.sayform input, .banform input { flex: 1; }
	.banform { border-top: none; padding: 0 0 0.5rem 0; }
	.banspane table { font-size: 12px; }
	.empty { color: #5a646c; padding: 0.5rem; }
	.right { display: flex; flex-direction: column; min-height: 0; }
	.tabs { display: flex; border-bottom: 1px solid #232b33; }
	.tabs button {
		background: none;
		border: none;
		border-bottom: 2px solid transparent;
		color: #8a949c;
		padding: 0.35rem 0.9rem;
		cursor: pointer;
		font: inherit;
	}
	.tabs button.active { color: #cdd6dd; border-bottom-color: #e8a33d; }
	.log { flex: 1; overflow-y: auto; padding: 0.4rem 0.6rem; }
	.line { white-space: pre-wrap; word-break: break-all; line-height: 1.45; }
	.line.kill { color: #e06c60; }
	.line.chat { color: #98c379; }
	.line.conn { color: #61afef; }
	.line.rcon { color: #c678dd; }
	.chatlog .msg { color: #98c379; }
	footer { border-top: 1px solid #232b33; background: #14191f; }
	.console { max-height: 180px; overflow-y: auto; padding: 0.3rem 0.6rem; }
	.console .cmd { color: #e8a33d; }
	.console pre { margin: 0 0 0.4rem 0; white-space: pre-wrap; color: #9aa5ad; }
	footer form { padding: 0.4rem 0.6rem; }
	input, select {
		background: #0c1014;
		border: 1px solid #2a333c;
		color: #cdd6dd;
		padding: 0.35rem 0.5rem;
		font: inherit;
	}
	footer input, .tokenform input { flex: 1; }
	button {
		background: #23415a;
		border: 1px solid #33587a;
		color: #cdd6dd;
		padding: 0.35rem 0.8rem;
		font: inherit;
		cursor: pointer;
	}
	button:hover { background: #2b4f6e; }
	button:disabled { opacity: 0.45; cursor: default; }
</style>
