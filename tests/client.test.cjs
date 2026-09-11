const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

// A small DOM fixture with real textContent replacement semantics. The client
// runs unchanged; drawing and network polling are outside these UI regressions.
class Element {
    constructor(text = '') {
        this.children = [];
        this.text = text;
        this.style = {};
        this.dataset = {};
        this.classes = new Set();
        this.classList = { toggle: (name, on) => on ? this.classes.add(name) : this.classes.delete(name) };
    }
    get textContent() { return this.text + this.children.map(c => c.textContent).join(''); }
    set textContent(value) { this.text = String(value); this.children = []; }
    set innerHTML(value) { throw new Error('Unsafe HTML rendering: ' + value); }
    appendChild(child) { this.children.push(child); return child; }
    removeChild(child) { this.children.splice(this.children.indexOf(child), 1); }
    get firstChild() { return this.children[0]; }
}

function fixture() {
    const ids = {};
    for (const id of ['login', 'game', 'login-error', 'playerName', 'totalPlayers', 'messages-server', 'messages-player']) ids[id] = new Element();
    ids.login.style.display = 'block';
    ids.game.style.display = 'none';
    ids.playerName.value = 'Pilot';
    const labels = {};
    const teams = ['Fed', 'Rom', 'Kli', 'Ori'].map((name, i) => {
        const radio = ids['team' + name] = new Element();
        radio.value = String(1 << i);
        radio.checked = i === 0;
        const count = ids[name.toLowerCase() + 'Count'] = new Element('(0)');
        const label = labels['label[for="team' + name + '"]'] = new Element(name + ' ');
        label.appendChild(count);
        return radio;
    });
    const ships = Array.from({length: 6}, (_, i) => ({ value: String(i), checked: i === 2 }));
    const sockets = [];
    class Socket {
        static OPEN = 1;
        static CONNECTING = 0;
        constructor() { this.readyState = 0; this.sent = []; sockets.push(this); }
        open() { this.readyState = 1; this.onopen?.(); }
        send(value) { this.sent.push(JSON.parse(value)); }
        close() { this.readyState = 3; this.onclose?.(); }
        deliver(type, data, extra = {}) { this.onmessage?.({ data: JSON.stringify({ type, data, ...extra }) }); }
    }
    const timers = [];
    const context = vm.createContext({
        console, Date, WebSocket: Socket,
        window: { location: { pathname: '/game.html', protocol: 'http:', host: 'localhost' }, addEventListener() {} },
        document: {
            getElementById: id => ids[id] || null,
            querySelector: selector => labels[selector] || (selector === 'input[name="team"]:checked' ? teams.find(r => r.checked) : selector === 'input[name="ship"]:checked' ? ships.find(r => r.checked) : null),
            querySelectorAll: selector => selector === 'input[name="team"]' ? teams : ships,
            createElement: () => new Element(),
        },
        setTimeout: callback => timers.push(callback), clearTimeout() {},
        setInterval() {}, clearInterval() {},
    });
    vm.runInContext(fs.readFileSync(path.join(__dirname, '../static/netrek.js'), 'utf8'), context);
    vm.runInContext('init = async () => {}; updateDashboard = () => {}; updatePlayerList = () => {}; updateTeamStats = () => {};', context);
    const read = expression => vm.runInContext(expression, context);
    return { context, ids, labels, teams, ships, sockets, timers, read };
}

function accepted(f, slot = 0) {
    f.context.connect();
    const socket = f.sockets.at(-1);
    socket.open();
    socket.deliver('login_success', { player_id: slot });
    return socket;
}

test('team counts remain attached and team availability matches server balance', () => {
    const f = fixture();
    f.context.updateTeamDisplay({ total: 0, teams: { fed: 0, rom: 0, kli: 0, ori: 0 } });
    f.context.updateTeamDisplay({ total: 1, teams: { fed: 1, rom: 0, kli: 0, ori: 0 } });
    const label = f.labels['label[for="teamFed"]'];
    assert.equal(label.textContent, 'Fed (1)');
    assert.ok(label.children.includes(f.ids.fedCount));
    assert.equal(f.ids.teamFed.disabled, true);
    assert.ok(f.teams.some(r => r.checked && !r.disabled));
    f.context.updateTeamDisplay({ total: 4, teams: { fed: 1, rom: 1, kli: 1, ori: 1 } });
    assert.equal(f.ids.teamFed.disabled, false);
    assert.equal(label.textContent, 'Fed (1)');
});

test('rejected login stays in the lobby and can retry on the same socket', () => {
    const f = fixture();
    f.context.connect();
    const socket = f.sockets[0];
    socket.open();
    assert.equal(f.ids.login.style.display, 'block');
    socket.deliver('error', 'Team is full');
    assert.equal(f.ids.login.style.display, 'block');
    assert.equal(f.ids.game.style.display, 'none');
    assert.equal(f.ids['login-error'].textContent, 'Team is full');
    f.teams.forEach((r, i) => r.checked = i === 1);
    f.context.connect();
    assert.equal(f.sockets.length, 1);
    assert.equal(socket.sent.at(-1).data.team, 2);
    socket.deliver('login_success', { player_id: 3 });
    assert.equal(f.ids.login.style.display, 'none');
    assert.equal(f.ids.game.style.display, 'block');
    assert.equal(f.read('uiState.inOutfitScreen'), false);
});

test('an update preceding reconnect acknowledgement cannot reuse the old identity', () => {
    const f = fixture();
    accepted(f, 4);
    f.context.reconnect();
    const socket = f.sockets.at(-1);
    socket.open();
    assert.equal(f.read('gameState.myPlayerID'), -1);
    socket.deliver('update', { players: [], planets: [], torps: [], plasmas: [] });
    assert.equal(f.read('uiState.inOutfitScreen'), false);
    socket.deliver('login_success', { player_id: 1 });
    assert.equal(f.read('gameState.myPlayerID'), 1);
    assert.equal(f.ids.game.style.display, 'block');
    assert.equal(f.ids.login.style.display, 'none');
});

test('accepted outfit selections become reconnect credentials', () => {
    const f = fixture();
    const socket = accepted(f);
    f.context.showLoginScreenAfterReset();
    f.ids.playerName.value = 'NewPilot';
    f.teams.forEach((r, i) => r.checked = i === 2);
    f.ships.forEach((r, i) => r.checked = i === 5);
    f.context.connect();
    assert.equal(f.ids.login.style.display, 'block');
    socket.deliver('login_success', { player_id: 2 });
    f.context.reconnect();
    const replacement = f.sockets.at(-1);
    replacement.open();
    assert.deepEqual(replacement.sent.at(-1).data, { name: 'NewPilot', team: 4, ship: 5 });
});

test('chat punctuation and markup render as literal text', () => {
    const f = fixture();
    const text = "I'm ready & waiting <script>alert('xss')</script>";
    f.context.addMessage(text, 'all', null, null, 'messages-player');
    const message = f.ids['messages-player'].children[0];
    assert.ok(message.textContent.endsWith(text));
    assert.equal(message.children.length, 0);
});

test('slash entry in an editable chat field keeps its default text insertion', () => {
    const f = fixture();
    let prevented = false;
    f.context.handleDocumentKeyDown({ key: '/', target: { closest: () => ({}) }, preventDefault: () => prevented = true });
    assert.equal(prevented, false);
    f.read('handleKeyPress = () => {}');
    f.context.handleDocumentKeyDown({ key: '/', target: { closest: () => null }, preventDefault: () => prevented = true });
    assert.equal(prevented, true);
});


test('authoritative unassignment returns to lobby even if the old slot was reused', () => {
    const f = fixture();
    const socket = accepted(f, 0);
    socket.deliver('update', { players: [{id:0, name:'Replacement', status:2, team:2}], planets: [], torps: [], plasmas: [] }, {player_id:-1});
    assert.equal(f.read('gameState.myPlayerID'), -1);
    assert.equal(f.read('uiState.inOutfitScreen'), true);
    assert.equal(f.ids.login.style.display, 'block');
    assert.equal(f.ids.game.style.display, 'none');
});
