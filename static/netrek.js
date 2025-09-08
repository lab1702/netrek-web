// Netrek Web Client

// Team colors used throughout the game
const teamColors = {
    0: '#888888',  // Ind - gray
    1: '#ffff00',  // Fed - yellow
    2: '#ff0000',  // Rom - red  
    4: '#00ff00',  // Kli - green
    8: '#00ffff'   // Ori - cyan
};

// Visual constants for galactic map
const GALACTIC_DIM_ALPHA = 0.5;        // Alpha level for dimmed ships
const GALACTIC_NEUTRAL_GRAY = '#888';  // Neutral gray for cloaked enemies

// Update planet counter display
function updatePlanetCounter() {
    if (!gameState.planets) return;
    
    // Count planets by team
    const counts = {
        0: 0,  // Ind
        1: 0,  // Fed
        2: 0,  // Rom
        4: 0,  // Kli
        8: 0   // Ori
    };
    
    gameState.planets.forEach(planet => {
        if (planet && planet.owner !== undefined) {
            counts[planet.owner] = (counts[planet.owner] || 0) + 1;
        }
    });
    
    // Update the display
    const fedElement = document.getElementById('fed-planets');
    const romElement = document.getElementById('rom-planets');
    const kliElement = document.getElementById('kli-planets');
    const oriElement = document.getElementById('ori-planets');
    const indElement = document.getElementById('ind-planets');
    
    if (fedElement) fedElement.textContent = counts[1];
    if (romElement) romElement.textContent = counts[2];
    if (kliElement) kliElement.textContent = counts[4];
    if (oriElement) oriElement.textContent = counts[8];
    if (indElement) indElement.textContent = counts[0];
}

// Update team display with the given data
function updateTeamDisplay(data) {
            // Update total players display
            const totalElement = document.getElementById('totalPlayers');
            if (totalElement) {
                totalElement.textContent = `${data.total} player${data.total !== 1 ? 's' : ''} online`;
            }
            
            // Update team radio button labels with player counts
            const fedCount = document.getElementById('fedCount');
            const romCount = document.getElementById('romCount');
            const kliCount = document.getElementById('kliCount');
            const oriCount = document.getElementById('oriCount');
            
            if (fedCount) fedCount.textContent = `(${data.teams.fed})`;
            if (romCount) romCount.textContent = `(${data.teams.rom})`;
            if (kliCount) kliCount.textContent = `(${data.teams.kli})`;
            if (oriCount) oriCount.textContent = `(${data.teams.ori})`;
            
            // Highlight teams with fewer players for balance
            const counts = [data.teams.fed, data.teams.rom, data.teams.kli, data.teams.ori];
            const minCount = Math.min(...counts);
            const maxCount = Math.max(...counts);
            
            const teamLabels = [
                document.querySelector('label[for="teamFed"]'),
                document.querySelector('label[for="teamRom"]'),
                document.querySelector('label[for="teamKli"]'),
                document.querySelector('label[for="teamOri"]')
            ];
            
            const teamRadios = [
                document.getElementById('teamFed'),
                document.getElementById('teamRom'),
                document.getElementById('teamKli'),
                document.getElementById('teamOri')
            ];
            
            let needNewSelection = false;
            let firstAvailableIndex = -1;
            
            for (let i = 0; i < teamLabels.length; i++) {
                if (!teamLabels[i] || !teamRadios[i]) continue;
                const count = counts[i];
                // Remove any existing star
                teamLabels[i].innerHTML = teamLabels[i].innerHTML.replace(' ⭐', '');
                
                if (count === maxCount && maxCount > minCount) {
                    // This team has the most players - disable it
                    teamLabels[i].style.color = '#f88';
                    teamRadios[i].disabled = true;
                    teamLabels[i].style.opacity = '0.5';
                    teamLabels[i].style.cursor = 'not-allowed';
                    
                    // If this team is currently selected, we need to select another
                    if (teamRadios[i].checked) {
                        needNewSelection = true;
                    }
                } else {
                    // This team is available
                    teamRadios[i].disabled = false;
                    teamLabels[i].style.opacity = '1';
                    teamLabels[i].style.cursor = 'pointer';
                    
                    if (count === minCount) {
                        // This team has fewer players - suggest it
                        teamLabels[i].style.color = '#0f0';
                        teamLabels[i].innerHTML += ' ⭐';
                        if (firstAvailableIndex === -1) {
                            firstAvailableIndex = i;
                        }
                    } else {
                        teamLabels[i].style.color = '#0f0';
                        if (firstAvailableIndex === -1) {
                            firstAvailableIndex = i;
                        }
                    }
                }
            }
            
            // If the currently selected team is full, select the first available team
            if (needNewSelection && firstAvailableIndex !== -1) {
                teamRadios[firstAvailableIndex].checked = true;
            }
}

// Fetch and display team populations  
function updateTeamStats() {
    fetch('api/teams')
        .then(response => response.json())
        .then(data => updateTeamDisplay(data))
        .catch(error => {
            console.error('Failed to fetch team stats:', error);
            const totalElement = document.getElementById('totalPlayers');
            if (totalElement) {
                totalElement.textContent = 'Server offline';
            }
        });
}

// Update team stats on page load and periodically
window.addEventListener('DOMContentLoaded', () => {
    updateTeamStats();
    // Update every 5 seconds while on login screen
    const statsInterval = setInterval(() => {
        if (document.getElementById('login').style.display !== 'none') {
            updateTeamStats();
        } else {
            clearInterval(statsInterval);
        }
    }, 5000);
});

let ws = null;
let wsCompressionActive = false;
let gameState = {
    myPlayerID: -1,
    players: [],
    planets: [],
    torps: [],
    plasmas: [],
    phasers: [], // Active phaser beams
    frame: 0,
    lastUpdate: 0,
    networkDelay: 0,
    interpolation: true,
    quitRequested: false // Track if player has requested to quit
};

// Store previous positions for interpolation
let prevState = {
    players: [],
    torps: [],
    plasmas: []
};

let controls = {
    mouseX: 0,
    mouseY: 0,
    keys: {}
};

let canvases = {
    tactical: null,
    galactic: null,
    tacticalCtx: null,
    galacticCtx: null
};

// Ship names
const shipNames = ['SC', 'DD', 'CA', 'BB', 'AS', 'SB', 'GA'];

// Player status constants (matching server-side types.go)
const StatusFree = 0;
const StatusOutfit = 1;
const StatusAlive = 2;
const StatusExplode = 3;
const StatusDead = 4;
const StatusObserve = 6;

// UI state tracking
let uiState = {
    inOutfitScreen: false
};

// Utility functions for team handling in victories
// Mirrors server-side logic from victory.go
function getTeamNamesFromFlag(teamFlag) {
    const names = [];
    if (teamFlag & 1) names.push('FEDERATION');  // TeamFed = 1
    if (teamFlag & 2) names.push('ROMULAN');     // TeamRom = 2
    if (teamFlag & 4) names.push('KLINGON');     // TeamKli = 4
    if (teamFlag & 8) names.push('ORION');       // TeamOri = 8
    return names;
}

// Get representative color for single or multiple teams
function getRepresentativeColor(teamFlag) {
    // For single team, return that team's color
    if (teamFlag === 1) return teamColors[1]; // Fed
    if (teamFlag === 2) return teamColors[2]; // Rom
    if (teamFlag === 4) return teamColors[4]; // Kli
    if (teamFlag === 8) return teamColors[8]; // Ori
    
    // For multiple teams or unknown, use neutral white
    return '#ffffff';
}

// Format team names for display (mirrors server formatTeamNames)
function formatTeamNames(names) {
    if (names.length === 0) return 'NO TEAMS';
    if (names.length === 1) return names[0];
    if (names.length === 2) return names[0] + ' & ' + names[1];
    
    // For 3+ teams, use commas with final "&"
    let result = '';
    for (let i = 0; i < names.length; i++) {
        if (i === names.length - 1) {
            result += ' & ' + names[i];
        } else if (i > 0) {
            result += ', ' + names[i];
        } else {
            result = names[i];
        }
    }
    return result;
}

// Initialize the game
async function init() {
    // Set up canvases
    canvases.tactical = document.getElementById('tactical');
    canvases.galactic = document.getElementById('galactic-map');
    canvases.tacticalCtx = canvases.tactical.getContext('2d');
    canvases.galacticCtx = canvases.galactic.getContext('2d');
    
    // Initialize planet renderer (but don't let it block canvas setup)
    if (window.planetRenderer) {
        try {
            await window.planetRenderer.init();
            // Planet renderer initialized
        } catch (err) {
            // Failed to initialize planet renderer
            // Continue without traditional planets
        }
    }
    
    // Initialize ship renderer
    if (window.shipRenderer) {
        try {
            await window.shipRenderer.init();
            // Ship renderer initialized
        } catch (err) {
            // Failed to initialize ship renderer
            // Continue without ship bitmaps
        }
    }
    
    // Resize canvases
    resizeCanvases();
    window.addEventListener('resize', resizeCanvases);
    
    // Set up input handlers
    setupInputHandlers();
    
    // Set up message input handlers
    const messageInput = document.getElementById('message-text');
    if (messageInput) {
        messageInput.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                sendChatMessage();
            } else if (e.key === 'Escape') {
                e.preventDefault();
                hideMessageInput();
            }
        });
    }
    
    // Start render loop at 10 FPS to match server tick rate
    setInterval(render, 100); // 100ms = 10 FPS
}

function resizeCanvases() {
    const dashboardHeight = 100;
    const padding = 40; // Account for padding and borders
    
    // Calculate the maximum square size that fits in the viewport
    const availableWidth = window.innerWidth - padding * 3; // Space for two squares plus gap
    const availableHeight = window.innerHeight - dashboardHeight - padding * 2;
    
    // Each canvas gets half the width (minus gap), but both need to fit vertically
    const maxSizeFromWidth = Math.floor(availableWidth / 2);
    const maxSizeFromHeight = availableHeight;
    
    // Use the smaller dimension to ensure squares fit
    const canvasSize = Math.min(maxSizeFromWidth, maxSizeFromHeight, 600); // Cap at 600px
    
    // Resizing canvases
    
    // Set both canvases to be perfect squares of the same size
    if (canvases.tactical) {
        canvases.tactical.width = canvasSize;
        canvases.tactical.height = canvasSize;
    }
    
    if (canvases.galactic) {
        canvases.galactic.width = canvasSize;
        canvases.galactic.height = canvasSize;
    }
}

function setupInputHandlers() {
    // Track which canvas the mouse is over
    controls.activeCanvas = 'tactical';
    controls.galacticMouseX = 0;
    controls.galacticMouseY = 0;
    
    // Mouse movement on tactical
    canvases.tactical.addEventListener('mousemove', (e) => {
        const rect = canvases.tactical.getBoundingClientRect();
        controls.mouseX = e.clientX - rect.left;
        controls.mouseY = e.clientY - rect.top;
        controls.activeCanvas = 'tactical';
    });
    
    // Mouse movement on galactic
    canvases.galactic.addEventListener('mousemove', (e) => {
        const rect = canvases.galactic.getBoundingClientRect();
        controls.galacticMouseX = e.clientX - rect.left;
        controls.galacticMouseY = e.clientY - rect.top;
        controls.activeCanvas = 'galactic';
    });
    
    // Mouse clicks - Netrek standard controls
    // Left button (0) = Torpedo
    // Middle button (1) = Phaser  
    // Right button (2) = Set course
    canvases.tactical.addEventListener('mousedown', (e) => {
        e.preventDefault();
        e.stopPropagation();
        
        if (gameState.myPlayerID < 0) return;
        const player = gameState.players[gameState.myPlayerID];
        if (!player || player.status !== 2) return; // Not alive
        
        // Calculate direction to mouse
        const centerX = canvases.tactical.width / 2;
        const centerY = canvases.tactical.height / 2;
        const dx = controls.mouseX - centerX;
        const dy = controls.mouseY - centerY;
        const dir = Math.atan2(dy, dx);
        
        switch(e.button) {
            case 0: // Left click - Fire torpedo
                sendMessage({
                    type: 'fire',
                    data: { dir: dir }
                });
                break;
                
            case 1: // Middle click - Fire phaser
                try {
                    // Send phaser command with direction - server handles targeting with original Netrek algorithm
                    sendMessage({
                        type: 'phaser',
                        data: { target: -1, dir: dir }
                    });
                } catch (err) {
                    // Error firing phaser
                }
                break;
                
            case 2: // Right click - Set course
                // Don't allow course changes while orbiting
                if (player.orbiting >= 0) {
                    // Ignore course change while orbiting
                    break;
                }
                // Set desired direction and maintain desired speed (not current speed)
                const desiredSpeed = player.desSpeed !== undefined ? player.desSpeed : player.speed || 0;
                sendMessage({
                    type: 'move',
                    data: { dir: dir, speed: desiredSpeed }
                });
                break;
        }
    });
    
    // Prevent context menu on right click
    canvases.tactical.addEventListener('contextmenu', (e) => {
        e.preventDefault();
        return false;
    });
    
    // Handle auxiliary click (middle button) explicitly
    canvases.tactical.addEventListener('auxclick', (e) => {
        e.preventDefault();
        e.stopPropagation();
        return false;
    });
    
    // Galactic map - only course setting, no weapons
    canvases.galactic.addEventListener('mousedown', (e) => {
        e.preventDefault();
        e.stopPropagation();
        
        if (gameState.myPlayerID < 0) return;
        const player = gameState.players[gameState.myPlayerID];
        if (!player || player.status !== 2) return; // Not alive
        
        // Only respond to right-click for course setting
        if (e.button !== 2) return;
        
        // Don't allow course changes while orbiting
        if (player.orbiting >= 0) return;
        
        // Get click position on galactic map
        const rect = canvases.galactic.getBoundingClientRect();
        const clickX = e.clientX - rect.left;
        const clickY = e.clientY - rect.top;
        
        // Convert click position to galaxy coordinates
        const width = canvases.galactic.width;
        const height = canvases.galactic.height;
        const scale = width / 100000; // Galaxy is 100000x100000
        
        const targetX = clickX / scale;
        const targetY = clickY / scale;
        
        // Calculate direction from player to target
        const dx = targetX - player.x;
        const dy = targetY - player.y;
        const dir = Math.atan2(dy, dx);
        
        // Set course maintaining desired speed
        const desiredSpeed = player.desSpeed !== undefined ? player.desSpeed : player.speed || 0;
        sendMessage({
            type: 'move',
            data: { dir: dir, speed: desiredSpeed }
        });
    });
    
    // Prevent context menu on galactic map right click
    canvases.galactic.addEventListener('contextmenu', (e) => {
        e.preventDefault();
        return false;
    });
    
    // Keyboard
    document.addEventListener('keydown', (e) => {
        controls.keys[e.key] = true;
        
        // Prevent Firefox Quick Find when pressing / for slash commands
        if (e.key === '/' && !e.ctrlKey && !e.altKey && !e.metaKey) {
            e.preventDefault();
        }
        
        handleKeyPress(e.key);
    });
    
    document.addEventListener('keyup', (e) => {
        controls.keys[e.key] = false;
    });
}

function handleKeyPress(key) {
    if (gameState.myPlayerID < 0) return;
    
    // Don't handle keys if typing a message
    if (document.getElementById('message-input').style.display === 'block') {
        return;
    }
    
    // Handle help window toggle first (works even when dead)
    if (key === '?') {
        const helpWindow = document.getElementById('help-window');
        if (helpWindow.style.display === 'block') {
            helpWindow.style.display = 'none';
        } else {
            helpWindow.style.display = 'block';
        }
        return;
    }
    
    // Handle escape key for closing windows
    if (key === 'Escape') {
        let windowClosed = false;
        
        // Close help window if open
        const helpWindow = document.getElementById('help-window');
        if (helpWindow.style.display === 'block') {
            helpWindow.style.display = 'none';
            windowClosed = true;
        }
        
        // Close info window if open
        if (window.infoWindow && window.infoWindow.destroy) {
            window.infoWindow.destroy();
            windowClosed = true;
        }
        
        if (windowClosed) {
            return;
        }
    }
    
    const player = gameState.players[gameState.myPlayerID];
    if (!player || player.status !== 2) return;
    
    // Speed control - numbers set speed
    if (key >= '0' && key <= '9') {
        const speed = parseInt(key);
        updateMovement(player, speed);
        return;
    }
    
    // Extended speed control for fast ships (10, 11, 12)
    if (key === '!' || key === '@' || key === '#') {
        let speed = 0;
        if (key === '!') speed = 10;      // Shift+1 = speed 10
        else if (key === '@') speed = 11; // Shift+2 = speed 11
        else if (key === '#') speed = 12; // Shift+3 = speed 12
        updateMovement(player, speed);
        return;
    }
    
    // Handle capital Q for quit/self-destruct (before toLowerCase)
    if (key === 'Q') {
        if (confirm('Self destruct? This will destroy your ship and disconnect you.')) {
            sendMessage({ type: 'quit', data: {} });
            gameState.quitRequested = true; // Track that we've requested to quit
        }
        return;
    }
    
    // Other keyboard commands (no direction control - that's mouse only!)
    switch(key.toLowerCase()) {
        case '\\':
        case '|':
            // Toggle practice panel (backslash key)
            const panel = document.getElementById('practice-panel');
            if (panel) {
                panel.classList.toggle('show');
            }
            break;
        case 's':
            sendMessage({ type: 'shields', data: {} });
            break;
        case 'c':
            sendMessage({ type: 'cloak', data: {} });
            break;
        case 'd':
            sendMessage({ type: 'detonate', data: {} });
            break;
        case 'y':
            // Find nearest enemy for pressor beam
            let nearestPressor = -1;
            let nearestPressorDist = 5000;
            for (let i = 0; i < gameState.players.length; i++) {
                const other = gameState.players[i];
                if (other && other.status === 2 && other.team !== player.team) {
                    const dist = Math.sqrt(
                        Math.pow(other.x - player.x, 2) + 
                        Math.pow(other.y - player.y, 2)
                    );
                    if (dist < nearestPressorDist) {
                        nearestPressorDist = dist;
                        nearestPressor = i;
                    }
                }
            }
            if (nearestPressor >= 0) {
                sendMessage({ type: 'pressor', data: { targetId: nearestPressor } });
            }
            break;
        case 'a':
            // All message
            showMessageInput('all');
            break;
        case '/':
            // Slash command shortcut - open All message window with '/' pre-filled
            showMessageInput('all', '/');
            break;
        case 't':
            // Check if shift is held for Team message
            if (controls.keys['Shift']) {
                showMessageInput('team');
            } else {
                // Find nearest enemy for tractor beam
                let nearestEnemy = -1;
                let nearestDist = 5000;
                for (let i = 0; i < gameState.players.length; i++) {
                    const other = gameState.players[i];
                    if (other && other.status === 2 && other.team !== player.team) {
                        const dist = Math.sqrt(
                            Math.pow(other.x - player.x, 2) + 
                            Math.pow(other.y - player.y, 2)
                        );
                        if (dist < nearestDist) {
                            nearestDist = dist;
                            nearestEnemy = i;
                        }
                    }
                }
                if (nearestEnemy >= 0) {
                    sendMessage({ type: 'tractor', data: { targetId: nearestEnemy } });
                }
            }
            break;
        case 'o':
            // Orbit planet
            sendMessage({ type: 'orbit', data: {} });
            break;
        case 'i':
        case 'I':
            // Show info window for nearest object
            showInfoWindow();
            break;
        case 'r':
        case 'R':
            // Toggle repair mode
            sendMessage({ type: 'repair', data: {} });
            break;
        case 'l':
            // Lock on to nearest planet to mouse cursor (no player locking)
            if (gameState.myPlayerID >= 0) {
                const myPlayer = gameState.players[gameState.myPlayerID];
                if (myPlayer) {
                    let closestPlanet = null;
                    let closestDist = Infinity;
                    let mouseX, mouseY;
                    
                    // Get world coordinates from mouse position based on active canvas
                    if (controls.activeCanvas === 'galactic') {
                        // Mouse is on galactic map
                        const scale = canvases.galactic.width / 100000;
                        mouseX = controls.galacticMouseX / scale;
                        mouseY = controls.galacticMouseY / scale;
                    } else {
                        // Mouse is on tactical map
                        const scale = canvases.tactical.width / 40000;
                        const centerX = canvases.tactical.width / 2;
                        const centerY = canvases.tactical.height / 2;
                        mouseX = myPlayer.x + (controls.mouseX - centerX) / scale;
                        mouseY = myPlayer.y + (controls.mouseY - centerY) / scale;
                    }
                    
                    // Check planets - find closest to mouse position
                    for (let i = 0; i < gameState.planets.length; i++) {
                        const planet = gameState.planets[i];
                        if (!planet) continue;
                        
                        const dist = Math.sqrt(
                            Math.pow(planet.x - mouseX, 2) + 
                            Math.pow(planet.y - mouseY, 2)
                        );
                        
                        if (dist < closestDist) {
                            closestDist = dist;
                            closestPlanet = { type: 'planet', target: i };
                        }
                    }
                    
                    if (closestPlanet) {
                        // If already locked on this planet, clear lock
                        if (myPlayer.lockType === 'planet' && 
                            myPlayer.lockTarget === closestPlanet.target) {
                            sendMessage({ type: 'lock', data: { type: 'none', target: -1 } });
                        } else {
                            sendMessage({ type: 'lock', data: closestPlanet });
                        }
                    }
                }
            }
            break;
        case 'z':
            // Beam up armies
            sendMessage({ type: 'beam', data: { up: true } });
            break;
        case 'x':
            // Beam down armies
            sendMessage({ type: 'beam', data: { up: false } });
            break;
        case 'b':
            // Bomb planet
            sendMessage({ type: 'bomb', data: {} });
            break;
        case 'p':
            // Fire plasma torpedo (use mouse direction)
            if (gameState.myPlayerID >= 0) {
                const myPlayer = gameState.players[gameState.myPlayerID];
                if (myPlayer) {
                    const centerX = canvases.tactical.width / 2;
                    const centerY = canvases.tactical.height / 2;
                    const dx = controls.mouseX - centerX;
                    const dy = controls.mouseY - centerY;
                    const dir = Math.atan2(dy, dx);
                    sendMessage({ type: 'plasma', data: { dir: dir } });
                }
            }
            break;
    }
}

function updateMovement(player, desiredSpeed) {
    if (!player) return;
    
    // Update speed only, keep current direction
    // Keep the current desired direction when changing speed
    sendMessage({
        type: 'move',
        data: { dir: player.desDir || player.dir || 0, speed: desiredSpeed }
    });
}

// Show login screen after game reset (return to team/ship selection)
function showLoginScreenAfterReset() {
    // Prevent duplicate calls
    if (uiState.inOutfitScreen) return;
    uiState.inOutfitScreen = true;
    
    console.log('Returning to team/ship selection after game reset');
    
    // Hide game interface, show login screen
    document.getElementById('game').style.display = 'none';
    document.getElementById('login').style.display = 'block';
    
    // Clear victory overlay state
    gameState.gameOver = false;
    
    // Get current player to pre-select their team
    const myPlayer = gameState.players[gameState.myPlayerID];
    if (myPlayer && myPlayer.team) {
        // Pre-select the radio button for current team
        const teamValue = myPlayer.team;
        const teamRadio = document.querySelector(`input[name="team"][value="${teamValue}"]`);
        if (teamRadio) {
            teamRadio.checked = true;
        }
        
        // Pre-select the radio button for current ship
        const shipValue = myPlayer.ship;
        const shipRadio = document.querySelector(`input[name="ship"][value="${shipValue}"]`);
        if (shipRadio) {
            shipRadio.checked = true;
        }
    }
    
    // Update team counts for lobby display
    updateTeamStats();
}

// Rejoin game with new team/ship selection (reuse existing WebSocket)
function reOutfit() {
    let name = document.getElementById('playerName').value || 'Player';
    
    // Validate and sanitize player name
    name = name.replace(/[^a-zA-Z0-9]/g, '').substring(0, 16);
    if (!name) name = 'Player';
    
    const team = parseInt(document.querySelector('input[name="team"]:checked').value);
    const ship = parseInt(document.querySelector('input[name="ship"]:checked').value);
    
    console.log('Rejoining game with team:', team, 'ship:', ship);
    
    // Hide login, show game
    document.getElementById('login').style.display = 'none';
    document.getElementById('game').style.display = 'block';
    uiState.inOutfitScreen = false;
    
    // Send outfit message to rejoin with new selection
    sendMessage({
        type: 'login', // Server expects 'login' type for both initial and rejoin
        data: { name: name, team: team, ship: ship }
    });
}

function connect() {
    // If already connected and in outfit screen, rejoin instead of reconnecting
    if (ws && ws.readyState === WebSocket.OPEN && uiState.inOutfitScreen) {
        reOutfit();
        return;
    }
    
    let name = document.getElementById('playerName').value || 'Player';
    
    // Validate and sanitize player name
    // Remove any non-alphanumeric characters and limit to 16 characters
    name = name.replace(/[^a-zA-Z0-9]/g, '').substring(0, 16);
    if (!name) name = 'Player'; // Default if name becomes empty after sanitization
    
    const team = parseInt(document.querySelector('input[name="team"]:checked').value);
    const ship = parseInt(document.querySelector('input[name="ship"]:checked').value);
    
    // Hide login, show game
    document.getElementById('login').style.display = 'none';
    document.getElementById('game').style.display = 'block';
    
    // Update compression indicator immediately
    updateCompressionIndicator();
    
    // Initialize game (with async handling)
    init().then(() => {
        // Game initialized successfully
    }).catch(err => {
        // Failed to initialize game
    });
    
    // Connect to WebSocket
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    // Get the base directory path (excluding the HTML file)
    let basePath = window.location.pathname;
    // If it ends with .html, get the directory path
    if (basePath.endsWith('.html')) {
        basePath = basePath.substring(0, basePath.lastIndexOf('/'));
    }
    // Remove trailing slashes and construct WebSocket path
    basePath = basePath.replace(/\/+$/, '');
    const wsPath = basePath ? `${basePath}/ws` : '/ws';
    ws = new WebSocket(`${protocol}//${window.location.host}${wsPath}`);
    
    ws.onopen = () => {
        // Connected to server
        // Check if compression is enabled by examining the WebSocket extensions
        if (ws.extensions && ws.extensions.includes('permessage-deflate')) {
            wsCompressionActive = true;
            console.log('WebSocket compression is ACTIVE (permessage-deflate)');
        } else {
            wsCompressionActive = false;
            console.log('WebSocket compression is NOT active');
        }
        updateCompressionIndicator();
        
        sendMessage({
            type: 'login',
            data: { name: name, team: team, ship: ship }
        });
    };
    
    ws.onmessage = (event) => {
        const msg = JSON.parse(event.data);
        handleServerMessage(msg);
    };
    
    ws.onerror = (error) => {
        // WebSocket error
        addMessage('Connection error!', 'warning');
    };
    
    ws.onclose = () => {
        // Disconnected from server
        addMessage('Disconnected from server', 'warning');
    };
}

function sendMessage(msg) {
    if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify(msg));
    }
}

function handleServerMessage(msg) {
    switch(msg.type) {
        case 'login_success':
            gameState.myPlayerID = msg.data.player_id;
            addMessage(`Joined as player ${msg.data.player_id}`, 'team');
            break;
            
        case 'update':
            // Store previous state for interpolation
            prevState.players = gameState.players.map(p => p ? {...p} : null);
            prevState.torps = gameState.torps.map(t => t ? {...t} : null);
            prevState.plasmas = gameState.plasmas.map(p => p ? {...p} : null);
            
            // Calculate network delay before updating lastUpdate
            const now = Date.now();
            if (gameState.lastUpdate) {
                gameState.networkDelay = now - gameState.lastUpdate;
            } else {
                gameState.networkDelay = 0;
            }
            
            gameState.frame = msg.data.frame;
            gameState.players = msg.data.players || [];
            gameState.planets = msg.data.planets || [];
            gameState.torps = msg.data.torps || [];
            gameState.plasmas = msg.data.plasmas || [];
            gameState.gameOver = msg.data.gameOver || false;
            gameState.winner = msg.data.winner;
            gameState.winType = msg.data.winType;
            
            // Update planet counter
            updatePlanetCounter();
            gameState.tMode = msg.data.tMode || false;
            gameState.tRemain = msg.data.tRemain;
            gameState.lastUpdate = now;
            
            // Update info window if it's visible
            if (window.infoWindow && window.infoWindow.isVisible()) {
                window.infoWindow.update();
            }
            
            // Check if player has quit and explosion has finished
            if (gameState.quitRequested && gameState.myPlayerID >= 0) {
                const myPlayer = gameState.players[gameState.myPlayerID];
                // Player status becomes 0 (StatusFree) after explosion completes
                if (!myPlayer || myPlayer.status === 0) {
                    // Redirect to login screen
                    window.location.href = 'index.html';
                    return;
                }
            }
            
            // Check if player should return to outfit screen after reset
            const myPlayer = gameState.players[gameState.myPlayerID];
            if (myPlayer && myPlayer.status === StatusOutfit && !uiState.inOutfitScreen) {
                showLoginScreenAfterReset();
                break; // Don't update dashboard/player list when returning to lobby
            }
            
            // Check if player slot was wiped (game reset) - player becomes null or StatusFree
            if (gameState.myPlayerID >= 0 && (!myPlayer || myPlayer.status === StatusFree) && !uiState.inOutfitScreen) {
                gameState.myPlayerID = -1; // Reset to lobby state
                showLoginScreenAfterReset();
                break; // Don't update dashboard/player list when returning to lobby
            }
            
            updateDashboard();
            updatePlayerList();
            break;
            
        case 'team_update':
            // Update team counts on login screen if visible
            if (document.getElementById('login').style.display !== 'none') {
                updateTeamDisplay(msg.data);
            }
            break;
            
        case 'message':
            // Handle death messages and other game messages
            const msgType = msg.data.type || '';
            const fromPlayer = msg.data.from !== undefined ? msg.data.from : null;
            const teamId = msg.data.team !== undefined ? msg.data.team : null;
            addMessage(msg.data.text, msgType, fromPlayer, teamId);
            
            // Play message sound for certain types
            if (msgType === 'kill' || msgType === 'death') {
            } else if (msgType === 'warning') {
            } else if (msgType !== 'info') {
            }
            break;
            
        case 'phaser':
            // Add phaser beam to render
            gameState.phasers.push({
                from: msg.data.from,
                to: msg.data.to,
                dir: msg.data.dir || 0, // Direction for missed phasers
                x: msg.data.x || 0,     // X coordinate for plasma hits
                y: msg.data.y || 0,     // Y coordinate for plasma hits
                range: msg.data.range || 5000, // Ship-specific phaser range, fallback to 5000
                life: 10 // Frames to display
            });
            // Play phaser sound when we see a phaser fired
            if (msg.data.from === gameState.myPlayerID || msg.data.to === gameState.myPlayerID) {
                // soundManager.play('phaser');  // TODO: implement sound
            }
            break;
            
        case 'error':
            addMessage(msg.data, 'warning', null, null);
            break;
    }
}

function lerp(start, end, t) {
    return start + (end - start) * t;
}

function getInterpolatedPosition(current, previous, entityId) {
    if (!gameState.interpolation || !previous || !current) {
        return current;
    }
    
    const now = Date.now();
    const timeSinceUpdate = now - gameState.lastUpdate;
    const updateInterval = 20; // 50 FPS = 20ms per frame
    const t = Math.min(timeSinceUpdate / updateInterval, 1);
    
    // Find previous position
    const prev = Array.isArray(previous) ? previous[entityId] : previous;
    if (!prev) return current;
    
    return {
        ...current,
        x: lerp(prev.x || 0, current.x || 0, t),
        y: lerp(prev.y || 0, current.y || 0, t),
        dir: current.dir // Don't interpolate direction
    };
}

function render() {
    // Calculate FPS
    frameCount++;
    const now = Date.now();
    if (now - lastFpsUpdate >= 1000) {
        fps = frameCount;
        frameCount = 0;
        lastFpsUpdate = now;
    }
    
    renderTactical();
    renderGalactic();
    // No longer calling requestAnimationFrame - using setInterval instead
}

function renderTactical() {
    // Don't render anything if player is in outfit screen
    if (uiState.inOutfitScreen) {
        return;
    }
    
    const ctx = canvases.tacticalCtx;
    const width = canvases.tactical.width;
    const height = canvases.tactical.height;
    
    // Clear
    ctx.fillStyle = '#000';
    ctx.fillRect(0, 0, width, height);
    
    // Draw alert level border (check early to get myPlayer reference)
    if (gameState.myPlayerID >= 0) {
        const myPlayer = gameState.players[gameState.myPlayerID];
        // Only draw alert border for yellow and red alert levels
        if (myPlayer && myPlayer.alertLevel && (myPlayer.alertLevel === 'yellow' || myPlayer.alertLevel === 'red')) {
        ctx.save();
        ctx.lineWidth = 3;
        
        // Set color based on alert level
        switch (myPlayer.alertLevel) {
            case 'red':
                ctx.strokeStyle = '#ff0000';
                ctx.shadowColor = '#ff0000';
                break;
            case 'yellow':
                ctx.strokeStyle = '#ffff00';
                ctx.shadowColor = '#ffff00';
                break;
        }
        
        ctx.shadowBlur = 8;
        
        // Draw border rectangle (adjusted for thinner border)
        ctx.strokeRect(2, 2, width - 4, height - 4);
        ctx.restore();
        }
    }
    
    // Tournament mode display moved to dashboard
    
    
    // Show victory screen if game is over
    if (gameState.gameOver) {
        const centerX = width / 2;
        const centerY = height / 2;
        
        ctx.save();
        ctx.fillStyle = 'rgba(0, 0, 0, 0.7)';
        ctx.fillRect(0, 0, width, height);
        
        // Use new utility functions to handle single and multi-team victories
        const winnerNames = getTeamNamesFromFlag(gameState.winner);
        const winnerText = formatTeamNames(winnerNames);
        const winnerColor = getRepresentativeColor(gameState.winner);
        
        ctx.fillStyle = winnerColor;
        ctx.font = 'bold 48px monospace';
        ctx.textAlign = 'center';
        
        // Choose appropriate victory text based on win type
        let winText = 'VICTORY!';
        if (gameState.winType === 'genocide') {
            winText = 'GENOCIDE VICTORY!';
        } else if (gameState.winType === 'conquest') {
            winText = 'CONQUEST VICTORY!';
        } else if (gameState.winType === 'domination') {
            winText = 'DOMINATION VICTORY!';
        } else if (gameState.winType === 'timeout') {
            winText = 'TIME LIMIT VICTORY!';
        }
        
        ctx.fillText(winText, centerX, centerY - 50);
        ctx.font = 'bold 36px monospace';
        
        // Handle plural vs singular for multiple winners
        const victoryText = winnerNames.length > 1 ? 
            `${winnerText} WIN!` : 
            `${winnerText} WINS!`;
        ctx.fillText(victoryText, centerX, centerY + 10);
        
        ctx.font = '20px monospace';
        ctx.fillStyle = '#fff';
        ctx.fillText('New game starting in 10 seconds...', centerX, centerY + 60);
        
        ctx.restore();
        return; // Don't render game elements during victory screen
    }
    
    // Draw stars
    ctx.fillStyle = '#444';
    for (let i = 0; i < 100; i++) {
        const x = (i * 137) % width;
        const y = (i * 89) % height;
        ctx.fillRect(x, y, 1, 1);
    }
    
    if (gameState.myPlayerID < 0) {
        return;
    }
    
    const myPlayer = gameState.players[gameState.myPlayerID];
    if (!myPlayer) {
        return;
    }
    
    const centerX = width / 2;
    const centerY = height / 2;
    const scale = 0.025; // Original Netrek scale: 40 units per pixel, 20000 units visible
    
    // Draw galaxy edges if visible
    ctx.save();
    ctx.strokeStyle = '#ff0000';
    ctx.lineWidth = 4;
    ctx.setLineDash([15, 10]); // Dashed line
    ctx.shadowColor = '#ff0000';
    ctx.shadowBlur = 5;
    
    // Calculate edge positions relative to player
    const leftEdge = (0 - myPlayer.x) * scale + centerX;
    const rightEdge = (100000 - myPlayer.x) * scale + centerX;
    const topEdge = (0 - myPlayer.y) * scale + centerY;
    const bottomEdge = (100000 - myPlayer.y) * scale + centerY;
    
    // Debug: Log player position and edge calculations when near edge
    if (myPlayer.x < 10000 || myPlayer.x > 90000 || myPlayer.y < 10000 || myPlayer.y > 90000) {
        // Check player position
    }
    
    // Draw edges if they're visible - check with some margin
    if (leftEdge >= -10 && leftEdge <= width + 10) {
        ctx.beginPath();
        ctx.moveTo(leftEdge, 0);
        ctx.lineTo(leftEdge, height);
        ctx.stroke();
    }
    if (rightEdge >= -10 && rightEdge <= width + 10) {
        ctx.beginPath();
        ctx.moveTo(rightEdge, 0);
        ctx.lineTo(rightEdge, height);
        ctx.stroke();
    }
    if (topEdge >= -10 && topEdge <= height + 10) {
        ctx.beginPath();
        ctx.moveTo(0, topEdge);
        ctx.lineTo(width, topEdge);
        ctx.stroke();
    }
    if (bottomEdge >= -10 && bottomEdge <= height + 10) {
        ctx.beginPath();
        ctx.moveTo(0, bottomEdge);
        ctx.lineTo(width, bottomEdge);
        ctx.stroke();
    }
    
    ctx.restore();
    
    // Draw planets in view
    for (const planet of gameState.planets) {
        if (!planet) continue;
        
        const dx = (planet.x - myPlayer.x) * scale;
        const dy = (planet.y - myPlayer.y) * scale;
        const screenX = centerX + dx;
        const screenY = centerY + dy;
        
        // Check if in view
        if (screenX < -50 || screenX > width + 50 || 
            screenY < -50 || screenY > height + 50) continue;
        
        // Use traditional planet renderer if available
        if (window.planetRenderer && window.planetRenderer.initialized) {
            window.planetRenderer.drawTacticalPlanet(ctx, planet, screenX, screenY);
        } else {
            // Fallback to gradient rendering
            const planetRadius = 20;
            const gradient = ctx.createRadialGradient(
                screenX - planetRadius/3, screenY - planetRadius/3, 0,
                screenX, screenY, planetRadius
            );
            
            const baseColor = teamColors[planet.owner];
            if (baseColor) {
                // Owned planet - team colored
                if (baseColor === '#ff0') { // Fed
                    gradient.addColorStop(0, '#ffd');
                    gradient.addColorStop(0.5, '#ff0');
                    gradient.addColorStop(1, '#cc0');
                } else if (baseColor === '#f00') { // Rom
                    gradient.addColorStop(0, '#fcc');
                    gradient.addColorStop(0.5, '#f00');
                    gradient.addColorStop(1, '#800');
                } else if (baseColor === '#0f0') { // Kli
                    gradient.addColorStop(0, '#cfc');
                    gradient.addColorStop(0.5, '#0f0');
                    gradient.addColorStop(1, '#080');
                } else if (baseColor === '#0ff') { // Ori
                    gradient.addColorStop(0, '#cff');
                    gradient.addColorStop(0.5, '#0ff');
                    gradient.addColorStop(1, '#088');
                }
            } else {
                // Neutral planet - gray
                gradient.addColorStop(0, '#bbb');
                gradient.addColorStop(0.5, '#888');
                gradient.addColorStop(1, '#444');
            }
            
            ctx.fillStyle = gradient;
            ctx.beginPath();
            ctx.arc(screenX, screenY, planetRadius, 0, Math.PI * 2);
            ctx.fill();
            
            // Planet border
            ctx.strokeStyle = teamColors[planet.owner] || '#666';
            ctx.lineWidth = 2;
            ctx.beginPath();
            ctx.arc(screenX, screenY, planetRadius, 0, Math.PI * 2);
            ctx.stroke();
            ctx.lineWidth = 1;
            
            // Planet name
            ctx.fillStyle = teamColors[planet.owner] || '#888';
            ctx.font = '10px monospace';
            ctx.textAlign = 'center';
            ctx.fillText(planet.name, screenX, screenY + 30);
        }
        
        // Orbit indicator removed - no visual indication when orbiting
        // if (myPlayer.orbiting === planet.id) {
        //     ctx.strokeStyle = '#0f0';
        //     ctx.setLineDash([2, 2]);
        //     ctx.beginPath();
        //     ctx.arc(screenX, screenY, 35, 0, Math.PI * 2);
        //     ctx.stroke();
        //     ctx.setLineDash([]);
        // }
    }
    
    // Draw phaser beams
    gameState.phasers = gameState.phasers.filter(phaser => {
        if (phaser.life <= 0) return false;
        
        const fromPlayer = gameState.players[phaser.from];
        if (!fromPlayer) return false;
        
        const fromX = centerX + (fromPlayer.x - myPlayer.x) * scale;
        const fromY = centerY + (fromPlayer.y - myPlayer.y) * scale;
        let toX, toY;
        
        if (phaser.to >= 0) {
            // Phaser hit a player target
            const toPlayer = gameState.players[phaser.to];
            if (!toPlayer) return false;
            toX = centerX + (toPlayer.x - myPlayer.x) * scale;
            toY = centerY + (toPlayer.y - myPlayer.y) * scale;
        } else if (phaser.to === -2) {
            // Phaser hit a plasma torpedo (special code -2)
            toX = centerX + (phaser.x - myPlayer.x) * scale;
            toY = centerY + (phaser.y - myPlayer.y) * scale;
        } else {
            // Phaser missed - draw in direction fired
            const phaserRange = (phaser.range || 5000) * scale; // Use ship-specific phaser range in screen pixels
            toX = fromX + Math.cos(phaser.dir) * phaserRange;
            toY = fromY + Math.sin(phaser.dir) * phaserRange;
        }
        
        // Draw phaser beam with gradient
        const gradient = ctx.createLinearGradient(fromX, fromY, toX, toY);
        let color = teamColors[fromPlayer.team] || '#fff';
        
        // Expand 3-char color to 6-char format if needed
        if (color.length === 4) {
            color = '#' + color[1] + color[1] + color[2] + color[2] + color[3] + color[3];
        }
        
        gradient.addColorStop(0, color);
        gradient.addColorStop(0.3, color + 'cc');
        gradient.addColorStop(0.7, color + '66');
        gradient.addColorStop(1, color + '00');
        
        ctx.strokeStyle = gradient;
        ctx.lineWidth = 3 + phaser.life / 3;
        ctx.globalAlpha = phaser.life / 10;
        ctx.beginPath();
        ctx.moveTo(fromX, fromY);
        ctx.lineTo(toX, toY);
        ctx.stroke();
        
        // Add glow effect
        ctx.strokeStyle = '#fff';
        ctx.lineWidth = 1;
        ctx.globalAlpha = phaser.life / 20;
        ctx.beginPath();
        ctx.moveTo(fromX, fromY);
        ctx.lineTo(toX, toY);
        ctx.stroke();
        
        // Add hit effect if phaser hit a target
        if (phaser.to >= 0 && phaser.life > 8) {
            // Draw impact flash
            ctx.fillStyle = color;
            ctx.globalAlpha = phaser.life / 15;
            ctx.beginPath();
            ctx.arc(toX, toY, 15 - (10 - phaser.life) * 3, 0, Math.PI * 2);
            ctx.fill();
            
            // Draw spark particles
            const numSparks = 4;
            for (let i = 0; i < numSparks; i++) {
                const angle = (Math.PI * 2 * i) / numSparks + phaser.life * 0.2;
                const dist = (10 - phaser.life) * 3;
                ctx.fillStyle = '#fff';
                ctx.globalAlpha = phaser.life / 15;
                ctx.fillRect(
                    toX + Math.cos(angle) * dist - 1,
                    toY + Math.sin(angle) * dist - 1,
                    2, 2
                );
            }
        }
        
        ctx.globalAlpha = 1;
        phaser.life--;
        return true;
    });
    
    // Draw torpedoes
    for (const torp of gameState.torps) {
        if (!torp) continue;
        
        const dx = (torp.x - myPlayer.x) * scale;
        const dy = (torp.y - myPlayer.y) * scale;
        const screenX = centerX + dx;
        const screenY = centerY + dy;
        
        if (screenX < 0 || screenX > width || screenY < 0 || screenY > height) continue;
        
        // Show explosion effect only when torpedo hits something (status = 3)
        // Do not show explosion when torpedo simply expires (fuse = 1)
        if (torp.status === 3) {
            ctx.fillStyle = '#ff0';
            ctx.globalAlpha = 0.8;
            ctx.beginPath();
            ctx.arc(screenX, screenY, 8, 0, Math.PI * 2);
            ctx.fill();
            ctx.globalAlpha = 0.4;
            ctx.beginPath();
            ctx.arc(screenX, screenY, 12, 0, Math.PI * 2);
            ctx.fill();
            ctx.globalAlpha = 1;
        } else {
            ctx.fillStyle = teamColors[torp.team] || '#888';
            ctx.fillRect(screenX - 2, screenY - 2, 4, 4);
        }
    }
    
    // Draw plasma torpedoes
    for (const plasma of gameState.plasmas) {
        if (!plasma) continue;
        
        const dx = (plasma.x - myPlayer.x) * scale;
        const dy = (plasma.y - myPlayer.y) * scale;
        const screenX = centerX + dx;
        const screenY = centerY + dy;
        
        if (screenX < -20 || screenX > width + 20 || screenY < -20 || screenY > height + 20) continue;
        
        // Plasma appears as larger, pulsing energy ball
        const pulseSize = 8 + Math.sin(gameState.frame * 0.2) * 3;
        
        // Outer glow
        ctx.fillStyle = teamColors[plasma.team] || '#888';
        ctx.globalAlpha = 0.3;
        ctx.beginPath();
        ctx.arc(screenX, screenY, pulseSize * 2, 0, Math.PI * 2);
        ctx.fill();
        
        // Middle ring
        ctx.globalAlpha = 0.6;
        ctx.beginPath();
        ctx.arc(screenX, screenY, pulseSize * 1.2, 0, Math.PI * 2);
        ctx.fill();
        
        // Core
        ctx.fillStyle = '#fff';
        ctx.globalAlpha = 1;
        ctx.beginPath();
        ctx.arc(screenX, screenY, pulseSize * 0.5, 0, Math.PI * 2);
        ctx.fill();
    }
    
    // Draw tractor/pressor beams
    for (let i = 0; i < gameState.players.length; i++) {
        const player = gameState.players[i];
        if (!player || player.status !== 2) continue;
        
        // Draw tractor beam
        if (player.tractoring >= 0 && player.tractoring < gameState.players.length) {
            const target = gameState.players[player.tractoring];
            if (target && target.status === 2) {
                const startX = centerX + (player.x - myPlayer.x) * scale;
                const startY = centerY + (player.y - myPlayer.y) * scale;
                const endX = centerX + (target.x - myPlayer.x) * scale;
                const endY = centerY + (target.y - myPlayer.y) * scale;
                
                // Draw tractor beam (blue)
                ctx.strokeStyle = '#00f';
                ctx.globalAlpha = 0.6;
                ctx.lineWidth = 2;
                ctx.setLineDash([10, 5]);
                ctx.beginPath();
                ctx.moveTo(startX, startY);
                ctx.lineTo(endX, endY);
                ctx.stroke();
                ctx.setLineDash([]);
                ctx.globalAlpha = 1;
            }
        }
        
        // Draw pressor beam
        if (player.pressoring >= 0 && player.pressoring < gameState.players.length) {
            const target = gameState.players[player.pressoring];
            if (target && target.status === 2) {
                const startX = centerX + (player.x - myPlayer.x) * scale;
                const startY = centerY + (player.y - myPlayer.y) * scale;
                const endX = centerX + (target.x - myPlayer.x) * scale;
                const endY = centerY + (target.y - myPlayer.y) * scale;
                
                // Draw pressor beam (orange)
                ctx.strokeStyle = '#f80';
                ctx.globalAlpha = 0.6;
                ctx.lineWidth = 2;
                ctx.setLineDash([5, 10]);
                ctx.beginPath();
                ctx.moveTo(startX, startY);
                ctx.lineTo(endX, endY);
                ctx.stroke();
                ctx.setLineDash([]);
                ctx.globalAlpha = 1;
            }
        }
    }
    
    // Draw players with interpolation
    for (let i = 0; i < gameState.players.length; i++) {
        const player = getInterpolatedPosition(gameState.players[i], prevState.players, i);
        if (!player) continue;
        
        // Skip free/outfit players
        if (player.status === 0 || player.status === 1) continue;
        
        // Handle explosion animation (status 3)
        if (player.status === 3) {
            const dx = (player.x - myPlayer.x) * scale;
            const dy = (player.y - myPlayer.y) * scale;
            const screenX = centerX + dx;
            const screenY = centerY + dy;
            
            if (screenX < -50 || screenX > width + 50 || 
                screenY < -50 || screenY > height + 50) continue;
            
            // Draw improved explosion with multiple effects
            const progress = 1 - (player.explodeTimer || 0) / 10;
            const maxSize = 80;
            
            // Multiple expanding rings
            for (let ring = 0; ring < 2; ring++) {
                const ringProgress = Math.min(1, progress * 1.5 - ring * 0.3);
                if (ringProgress > 0) {
                    ctx.strokeStyle = ring === 0 ? '#fff' : '#ff0';
                    ctx.lineWidth = 3 - ring;
                    ctx.globalAlpha = (1 - ringProgress) * 0.8;
                    ctx.beginPath();
                    ctx.arc(screenX, screenY, maxSize * ringProgress * (1 + ring * 0.3), 0, Math.PI * 2);
                    ctx.stroke();
                }
            }
            
            // Main explosion fireball
            const gradient = ctx.createRadialGradient(screenX, screenY, 0, screenX, screenY, maxSize * progress);
            gradient.addColorStop(0, 'rgba(255, 255, 255, 1)');
            gradient.addColorStop(0.2, 'rgba(255, 255, 100, 0.9)');
            gradient.addColorStop(0.4, 'rgba(255, 150, 0, 0.7)');
            gradient.addColorStop(0.7, 'rgba(255, 50, 0, 0.4)');
            gradient.addColorStop(1, 'rgba(255, 0, 0, 0)');
            
            ctx.fillStyle = gradient;
            ctx.globalAlpha = Math.pow(1 - progress, 0.5);
            ctx.beginPath();
            ctx.arc(screenX, screenY, maxSize * progress * 0.8, 0, Math.PI * 2);
            ctx.fill();
            
            // Bright core flash
            if (progress < 0.3) {
                ctx.fillStyle = '#fff';
                ctx.globalAlpha = (1 - progress * 3);
                ctx.beginPath();
                ctx.arc(screenX, screenY, maxSize * 0.2, 0, Math.PI * 2);
                ctx.fill();
            }
            
            // Debris particles with trails
            const numParticles = 16;
            for (let j = 0; j < numParticles; j++) {
                const angle = (j * Math.PI * 2 / numParticles) + progress * 0.5;
                const speed = 1 + (j % 3) * 0.3;
                const dist = maxSize * progress * speed;
                
                // Particle trail
                if (progress < 0.7) {
                    ctx.strokeStyle = j % 2 ? '#ff0' : '#f80';
                    ctx.lineWidth = 2 * (1 - progress);
                    ctx.globalAlpha = (1 - progress) * 0.6;
                    ctx.beginPath();
                    ctx.moveTo(screenX, screenY);
                    ctx.lineTo(screenX + Math.cos(angle) * dist, screenY + Math.sin(angle) * dist);
                    ctx.stroke();
                }
                
                // Glowing particle
                ctx.fillStyle = j % 3 === 0 ? '#fff' : j % 3 === 1 ? '#ff0' : '#f80';
                ctx.globalAlpha = 1 - progress;
                const particleSize = 4 * (1 - progress);
                ctx.fillRect(
                    screenX + Math.cos(angle) * dist - particleSize/2,
                    screenY + Math.sin(angle) * dist - particleSize/2,
                    particleSize, particleSize
                );
            }
            ctx.globalAlpha = 1;
            continue;
        }
        
        // Skip dead players (status 4)
        if (player.status !== 2) continue; // Not alive
        
        // Skip cloaked enemy ships entirely - they should be invisible
        if (player.cloaked && player.team !== myPlayer.team) {
            continue;
        }
        
        const dx = (player.x - myPlayer.x) * scale;
        const dy = (player.y - myPlayer.y) * scale;
        const screenX = centerX + dx;
        const screenY = centerY + dy;
        
        // Check if in view
        if (screenX < -20 || screenX > width + 20 || 
            screenY < -20 || screenY > height + 20) continue;
        
        // Draw ship
        ctx.save();
        
        // Make cloaked friendly ships translucent
        if (player.cloaked && player.team === myPlayer.team) {
            ctx.globalAlpha = GALACTIC_DIM_ALPHA;
        }
        
        // Draw ship using ship renderer or fallback
        if (window.shipRenderer && window.shipRenderer.initialized) {
            window.shipRenderer.drawShip(ctx, player, screenX, screenY, 1);
        } else {
            // Fallback to simple triangle
            ctx.translate(screenX, screenY);
            ctx.rotate(player.dir || 0);
            ctx.strokeStyle = teamColors[player.team] || '#fff';
            ctx.lineWidth = i === gameState.myPlayerID ? 2 : 1;
            
            // Mark bots with dashed lines
            if (player.isBot) {
                ctx.setLineDash([3, 3]);
            }
            
            ctx.beginPath();
            ctx.moveTo(10, 0);
            ctx.lineTo(-5, -5);
            ctx.lineTo(-5, 5);
            ctx.closePath();
            ctx.stroke();
        }
        
        // Shield circle if shields up
        if (player.shields_up) {
            ctx.save();
            ctx.translate(screenX, screenY);
            ctx.strokeStyle = teamColors[player.team] || '#fff';
            ctx.globalAlpha = 0.3;
            ctx.beginPath();
            ctx.arc(0, 0, 15, 0, Math.PI * 2);
            ctx.stroke();
            ctx.restore();
        }
        
        // Repair mode indicator - wrench icon or pulsing effect
        if (player.repairing) {
            ctx.save();
            ctx.translate(screenX, screenY);
            ctx.strokeStyle = '#0f0';
            ctx.globalAlpha = 0.5 + Math.sin(Date.now() / 200) * 0.3;
            ctx.lineWidth = 2;
            ctx.beginPath();
            ctx.arc(0, 0, 20, 0, Math.PI * 2);
            ctx.stroke();
            ctx.restore();
        }
        
        ctx.restore();
        
        // Draw ship type letters on the ship (commented out when using bitmaps)
        // Uncomment if you want ship letters over the bitmaps
        /*
        ctx.fillStyle = teamColors[player.team] || '#fff';
        ctx.font = 'bold 9px monospace';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        const shipType = shipNames[player.ship] || 'XX';
        ctx.fillText(shipType, screenX, screenY);
        */
        
        // Player label below (team letter + slot number)
        const teamLetters = {
            0: 'I',  // Ind
            1: 'F',  // Fed
            2: 'R',  // Rom
            4: 'K',  // Kli
            8: 'O'   // Ori
        };
        const teamLetter = teamLetters[player.team] || 'I';
        const playerLabel = teamLetter + i.toString().padStart(2, '0');
        
        ctx.fillStyle = teamColors[player.team] || '#fff';
        ctx.font = i === gameState.myPlayerID ? 'bold 9px monospace' : '9px monospace';
        ctx.textBaseline = 'top';
        ctx.fillText(playerLabel, screenX, screenY + 12);
    }
    
    // Lock indicator removed
    
    // Crosshair and aiming line removed
    ctx.globalAlpha = 1;
}

function renderGalactic() {
    const ctx = canvases.galacticCtx;
    const width = canvases.galactic.width;
    const height = canvases.galactic.height;
    
    // Clear
    ctx.fillStyle = '#000';
    ctx.fillRect(0, 0, width, height);
    
    // Scale to fit galaxy
    const scale = width / 100000;
    
    // Draw planets
    for (const planet of gameState.planets) {
        if (!planet) continue;
        
        const x = planet.x * scale;
        const y = planet.y * scale;
        
        // Use traditional planet renderer if available
        if (window.planetRenderer && window.planetRenderer.initialized) {
            window.planetRenderer.drawGalacticPlanet(ctx, planet, x, y);
        } else {
            // Fallback to text labels
            ctx.font = '9px monospace';
            ctx.textAlign = 'center';
            ctx.textBaseline = 'middle';
            ctx.fillStyle = teamColors[planet.owner] || '#888';
            ctx.fillText(planet.label || planet.name.substring(0, 3).toUpperCase(), x, y);
        }
    }
    
    // Map team IDs to letters
    const teamLetters = {
        0: 'I',  // Ind
        1: 'F',  // Fed
        2: 'R',  // Rom
        4: 'K',  // Kli
        8: 'O'   // Ori
    };
    
    // Draw players
    const myPlayer = gameState.myPlayerID >= 0 ? gameState.players[gameState.myPlayerID] : null;
    for (let i = 0; i < gameState.players.length; i++) {
        const player = gameState.players[i];
        if (!player) continue;
        
        const x = player.x * scale;
        const y = player.y * scale;
        
        // Show explosions on galactic map
        if (player.status === 3) {
            ctx.strokeStyle = teamColors[player.team] || '#f00';
            ctx.globalAlpha = 0.8;
            ctx.beginPath();
            ctx.arc(x, y, 5, 0, Math.PI * 2);
            ctx.stroke();
            ctx.globalAlpha = 1;
            continue;
        }
        
        if (player.status !== 2) continue; // Only show alive players
        
        // Show cloaked enemy ships as dimmed '??' on galactic map
        if (player.cloaked && myPlayer && player.team !== myPlayer.team) {
            ctx.save();
            ctx.globalAlpha = GALACTIC_DIM_ALPHA;
            ctx.fillStyle = GALACTIC_NEUTRAL_GRAY;
            ctx.font = '10px monospace';
            ctx.textAlign = 'center';
            ctx.textBaseline = 'middle';
            ctx.fillText('??', x, y);
            ctx.restore();
            continue;
        }
        
        // Save context for potential alpha changes
        ctx.save();
        
        // Make friendly cloaked ships translucent on galactic map
        if (player.cloaked && myPlayer && player.team === myPlayer.team) {
            ctx.globalAlpha = GALACTIC_DIM_ALPHA;
        }
        
        // Draw player as team letter + slot number (e.g., "R45")
        const teamLetter = teamLetters[player.team] || 'I';
        const playerLabel = teamLetter + i.toString().padStart(2, '0');
        
        ctx.fillStyle = teamColors[player.team] || '#fff';
        ctx.font = i === gameState.myPlayerID ? 'bold 10px monospace' : '9px monospace';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText(playerLabel, x, y);
        
        ctx.restore();
    }
    
    // Lock indicator on galactic map removed
}

let lastWarningTime = 0;

// Performance tracking
let fps = 0;
let frameCount = 0;
let lastFpsUpdate = 0;

function updateCompressionIndicator() {
    const indicator = document.getElementById('compression-indicator');
    if (indicator) {
        if (wsCompressionActive) {
            indicator.textContent = 'ON';
            indicator.style.color = '#0f0';
            indicator.title = 'WebSocket compression (permessage-deflate) is active - reduced bandwidth usage';
        } else {
            indicator.textContent = 'OFF';
            indicator.style.color = '#888';
            indicator.title = 'WebSocket compression is not active';
        }
    }
}

function updateDashboard() {
    if (gameState.myPlayerID < 0) return;
    
    const player = gameState.players[gameState.myPlayerID];
    if (!player) return;
    
    updateCompressionIndicator();
    
    // Update network delay
    const lag = gameState.networkDelay || 0;
    const delayEl = document.getElementById('network-delay');
    if (delayEl) {
        delayEl.textContent = `${lag}ms`;
        if (lag < 150) {
            delayEl.style.color = '#0f0';  // Green
        } else if (lag < 200) {
            delayEl.style.color = '#ff0';  // Yellow
        } else {
            delayEl.style.color = '#f00';  // Red
        }
    }
    
    // Update stats with current/max format where applicable
    const maxShields = getMaxShields(player.ship);
    const maxDamage = getMaxDamage(player.ship);
    const maxFuel = getMaxFuel(player.ship);
    const maxSpeed = getMaxSpeed(player.ship);
    const maxArmies = getMaxArmies(player.ship);
    
    document.getElementById('shields').textContent = `${player.shields || 0} / ${maxShields}`;
    document.getElementById('damage').textContent = `${player.damage || 0} / ${maxDamage}`;
    document.getElementById('fuel').textContent = `${player.fuel || 0} / ${maxFuel}`;
    document.getElementById('wtemp').textContent = player.wtemp || 0;
    document.getElementById('etemp').textContent = player.etemp || 0;
    document.getElementById('speed').textContent = `${Math.round(player.speed || 0)} / ${maxSpeed}`;
    
    // Update KS/K/D stats
    const killStreak = Math.floor(player.killsStreak || 0);
    const kills = Math.floor(player.kills || 0);
    const deaths = player.deaths || 0;
    const kdaEl = document.getElementById('kda-stats');
    if (kdaEl) {
        kdaEl.textContent = `${killStreak} / ${kills} / ${deaths}`;
        // Color based on kill streak
        if (killStreak >= 5) {
            kdaEl.style.color = '#ff0'; // Yellow for high streak
        } else if (killStreak >= 3) {
            kdaEl.style.color = '#0ff'; // Cyan for medium streak
        } else {
            kdaEl.style.color = '#8f8'; // Light green default
        }
    }
    
    // Update K/D ratio
    const kdRatioEl = document.getElementById('kd-ratio');
    if (kdRatioEl) {
        const kdRatio = deaths > 0 ? (kills / deaths).toFixed(2) : kills.toFixed(2);
        kdRatioEl.textContent = kdRatio;
        // Color based on K/D ratio
        if (parseFloat(kdRatio) >= 2.0) {
            kdRatioEl.style.color = '#ff0'; // Yellow for excellent
        } else if (parseFloat(kdRatio) >= 1.0) {
            kdRatioEl.style.color = '#0ff'; // Cyan for positive
        } else if (parseFloat(kdRatio) >= 0.5) {
            kdRatioEl.style.color = '#8f8'; // Light green for okay
        } else {
            kdRatioEl.style.color = '#f88'; // Light red for poor
        }
    }
    
    
    // Warning sounds (limit to once every 2 seconds)
    const now = Date.now();
    if (now - lastWarningTime > 2000) {
        // Critical fuel warning
        if (player.fuel < 1000 && player.fuel > 0) {
            // soundManager.play('warning');  // TODO: implement sound
            lastWarningTime = now;
        }
        // Critical damage warning
        else if (player.damage > 80) {
            // soundManager.play('warning');  // TODO: implement sound
            lastWarningTime = now;
        }
    }
    
    // Update armies and orbit status
    const armiesEl = document.getElementById('armies');
    if (armiesEl) {
        armiesEl.textContent = `${player.armies || 0} / ${maxArmies}`;
        // Gray out armies section if kill streak is less than 2
        if (killStreak < 2) {
            armiesEl.style.color = '#888'; // Gray
        } else {
            armiesEl.style.color = '#0f0'; // Green (normal)
        }
    }
    
    const statusEl = document.getElementById('status');
    if (statusEl) {
        statusEl.style.color = ''; // Reset color to default
        let statusText = '';
        if (player.orbiting >= 0 && gameState.planets[player.orbiting]) {
            const planet = gameState.planets[player.orbiting];
            statusText = `Orbiting ${planet.name}`;
            if (player.bombing) statusText += ' [BOMBING]';
            if (player.beaming) statusText += ' [BEAMING]';
        } else if (player.repairing) {
            statusText = 'REPAIR MODE';
        } else if (player.lockType === 'planet' && player.lockTarget >= 0) {
            const target = gameState.planets[player.lockTarget];
            if (target) {
                statusText = `Lock: ${target.name}`;
            }
        } else if (player.engineOverheat) {
            statusText = 'ENGINES OVERHEATED!';
            statusEl.style.color = '#f00'; // Make it red
        } else if (player.shields_up === false) {
            statusText = 'Shields Down';
        } else if (player.cloaked) {
            statusText = 'Cloaked';
        }
        statusEl.textContent = statusText;
    }
    
    // Update tournament mode display in planet counter
    const tournamentDisplay = document.getElementById('tournament-timer-display');
    const tournamentTimer = document.getElementById('tournament-timer-value');
    if (tournamentDisplay && tournamentTimer) {
        if (gameState.tMode && !gameState.gameOver) {
            tournamentDisplay.style.display = 'inline-block';
            const minutes = Math.floor(gameState.tRemain / 60);
            const seconds = gameState.tRemain % 60;
            tournamentTimer.textContent = `${minutes}:${seconds.toString().padStart(2, '0')}`;
        } else {
            tournamentDisplay.style.display = 'none';
        }
    }
    
    
    // Update alert status display
    updateAlertStatus();
}

function updateAlertStatus() {
    const alertEl = document.getElementById('alert-status');
    if (!alertEl) return;
    
    if (gameState.myPlayerID < 0) {
        alertEl.style.display = 'none';
        return;
    }
    
    const player = gameState.players[gameState.myPlayerID];
    if (!player || !player.alertLevel || player.alertLevel === 'green') {
        alertEl.style.display = 'none';
        return;
    }
    
    if (player.alertLevel === 'yellow') {
        alertEl.textContent = 'Yellow Alert';
        alertEl.className = 'yellow-alert';
        alertEl.style.display = 'block';
    } else if (player.alertLevel === 'red') {
        alertEl.textContent = 'RED ALERT';
        alertEl.className = 'red-alert';
        alertEl.style.display = 'block';
    } else {
        alertEl.style.display = 'none';
    }
}

function updatePlayerList() {
    const list = document.getElementById('player-list');
    let html = '<div style="border-bottom: 1px solid #808080; margin-bottom: 5px; display: flex; justify-content: space-between; font-size: 9px; color: #c0c0c0;"><span>ID   PLAYERS</span><span>KS/K/D/KD</span></div>';
    
    // Map team IDs to letters
    const teamLetters = {
        0: 'I',  // Ind
        1: 'F',  // Fed
        2: 'R',  // Rom
        4: 'K',  // Kli
        8: 'O'   // Ori
    };
    
    // Filter and collect visible players with their slot numbers
    const visiblePlayers = [];
    for (let i = 0; i < gameState.players.length; i++) {
        const player = gameState.players[i];
        // Show alive players (status 2) and dead but connected players (status 4)
        if (player && player.status !== 0 && player.status !== 1) {
            visiblePlayers.push({ ...player, slot: i });
        }
    }
    
    // Sort by team first, then by slot
    visiblePlayers.sort((a, b) => {
        // Sort by team first
        if (a.team !== b.team) {
            return a.team - b.team;
        }
        // Then sort by slot number
        return a.slot - b.slot;
    });
    
    for (const player of visiblePlayers) {
        const isDead = player.status === 4 || player.status === 3; // Dead or exploding
        const teamClass = `team-${getTeamName(player.team).toLowerCase()}`;
        const shipType = shipNames[player.ship] || 'XX';
        const kills = player.kills || 0;
        const killsStreak = player.killsStreak || 0;
        const deaths = player.deaths || 0;
        const kd = deaths > 0 ? (kills / deaths).toFixed(2) : kills.toFixed(1);
        
        // Create team/slot identifier
        const teamLetter = teamLetters[player.team] || 'I';
        const playerID = teamLetter + player.slot.toString().padStart(2, '0');
        
        // Add opacity style for dead players
        const deadStyle = isDead ? 'opacity: 0.4;' : '';
        
        html += `<div class="player-entry ${teamClass}" style="display: flex; justify-content: space-between; ${deadStyle}">
            <span><span style="font-family: monospace; margin-right: 4px;">${playerID}</span> ${player.name || 'Player'} (${shipType})</span>
            <span style="font-size: 9px;">${Math.floor(killsStreak)} / ${Math.floor(kills)} / ${deaths} / ${kd}</span>
        </div>`;
    }
    
    list.innerHTML = html;
}

let messageMode = '';

function showMessageInput(mode, initialText = '') {
    messageMode = mode;
    const inputDiv = document.getElementById('message-input');
    const prompt = inputDiv.querySelector('.prompt');
    const input = document.getElementById('message-text');
    
    if (mode === 'team') {
        prompt.textContent = 'Team message:';
    } else if (mode === 'all') {
        prompt.textContent = 'All message:';
    } else if (mode.startsWith('private:')) {
        const targetId = parseInt(mode.split(':')[1]);
        const target = gameState.players[targetId];
        prompt.textContent = `Private to ${target ? target.name : 'player'}:`;
    }
    
    inputDiv.style.display = 'block';
    input.value = initialText;
    // Use setTimeout to ensure the input is set after the current event finishes
    setTimeout(() => {
        input.value = initialText;
        input.focus();
        // Move cursor to the end of the text
        input.setSelectionRange(initialText.length, initialText.length);
    }, 0);
}

function hideMessageInput() {
    document.getElementById('message-input').style.display = 'none';
    messageMode = '';
}

function showInfoWindow() {
    // Check if info window is already visible - if so, close it
    if (window.infoWindow && window.infoWindow.isVisible()) {
        window.infoWindow.destroy();
        return;
    }
    
    // Find what's under the mouse cursor
    const myPlayer = gameState.myPlayerID >= 0 ? gameState.players[gameState.myPlayerID] : null;
    if (!myPlayer) return;
    
    let closestDistance = Infinity;
    let closestTarget = null;
    let targetType = null;
    
    // Get mouse position on the active canvas
    let mouseX, mouseY;
    if (controls.activeCanvas === 'tactical') {
        mouseX = controls.mouseX;
        mouseY = controls.mouseY;
        
        const canvas = canvases.tactical;
        const centerX = canvas.width / 2;
        const centerY = canvas.height / 2;
        const scale = canvas.width / 20000;
        
        // Check players
        for (let i = 0; i < gameState.players.length; i++) {
            const player = gameState.players[i];
            if (!player || player.status !== 2) continue;
            
            // Don't allow targeting cloaked enemies
            if (player.cloaked && player.team !== myPlayer.team) continue;
            
            const dx = (player.x - myPlayer.x) * scale;
            const dy = (player.y - myPlayer.y) * scale;
            const screenX = centerX + dx;
            const screenY = centerY + dy;
            
            const dist = Math.pow(screenX - mouseX, 2) + Math.pow(screenY - mouseY, 2);
            if (dist < closestDistance) {
                closestDistance = dist;
                closestTarget = player;
                targetType = 'player';
                closestTarget.playerIndex = i; // Store the player index for team/slot display
            }
        }
        
        // Check planets
        for (const planet of gameState.planets) {
            if (!planet) continue;
            
            const dx = (planet.x - myPlayer.x) * scale;
            const dy = (planet.y - myPlayer.y) * scale;
            const screenX = centerX + dx;
            const screenY = centerY + dy;
            
            const dist = Math.pow(screenX - mouseX, 2) + Math.pow(screenY - mouseY, 2);
            if (dist < closestDistance) {
                closestDistance = dist;
                closestTarget = planet;
                targetType = 'planet';
            }
        }
    } else {
        // Galactic map
        mouseX = controls.galacticMouseX;
        mouseY = controls.galacticMouseY;
        
        const canvas = canvases.galactic;
        const scale = canvas.width / 100000;
        
        // Check planets on galactic
        for (const planet of gameState.planets) {
            if (!planet) continue;
            
            const screenX = planet.x * scale;
            const screenY = planet.y * scale;
            
            const dist = Math.pow(screenX - mouseX, 2) + Math.pow(screenY - mouseY, 2);
            if (dist < closestDistance) {
                closestDistance = dist;
                closestTarget = planet;
                targetType = 'planet';
            }
        }
        
        // Check players on galactic
        for (let i = 0; i < gameState.players.length; i++) {
            const player = gameState.players[i];
            if (!player || player.status !== 2) continue;
            
            // Don't allow targeting cloaked enemies
            const myPlayer = gameState.players[gameState.myPlayerID];
            if (myPlayer && player.cloaked && player.team !== myPlayer.team) continue;
            
            const screenX = player.x * scale;
            const screenY = player.y * scale;
            
            const dist = Math.pow(screenX - mouseX, 2) + Math.pow(screenY - mouseY, 2);
            if (dist < closestDistance) {
                closestDistance = dist;
                closestTarget = player;
                targetType = 'player';
                closestTarget.playerIndex = i; // Store the player index for team/slot display
            }
        }
    }
    
    // Show info window if we found something close enough
    if (closestTarget && closestDistance < 2500) { // Within 50 pixels
        if (window.infoWindow) {
            // Get actual screen coordinates for window placement
            const rect = controls.activeCanvas === 'tactical' ? 
                canvases.tactical.getBoundingClientRect() : 
                canvases.galactic.getBoundingClientRect();
            const windowX = rect.left + mouseX + 20;
            const windowY = rect.top + mouseY;
            
            if (targetType === 'planet') {
                // For now, assume we have info on all planets we can see
                // In a real game, this would come from the server based on scouting
                if (closestTarget.info === undefined || closestTarget.info === 0) {
                    // Set info bit for our team (allows us to see planet details)
                    const myPlayer = gameState.players[gameState.myPlayerID];
                    if (myPlayer) {
                        closestTarget.info = myPlayer.team; // Set our team's bit
                    } else {
                        closestTarget.info = 15; // All teams for testing
                    }
                }
                window.infoWindow.showPlanetInfo(closestTarget, windowX, windowY);
            } else if (targetType === 'player') {
                const playerIndex = closestTarget.playerIndex !== undefined ? closestTarget.playerIndex : -1;
                window.infoWindow.showPlayerInfo(closestTarget, windowX, windowY, playerIndex);
            }
        }
    } else {
        // Close info window if clicking 'i' with nothing nearby
        if (window.infoWindow && window.infoWindow.isVisible()) {
            window.infoWindow.destroy();
        }
    }
}

function sendChatMessage() {
    const input = document.getElementById('message-text');
    const text = input.value.trim();
    
    if (!text) {
        hideMessageInput();
        return;
    }
    
    if (messageMode === 'team') {
        sendMessage({ type: 'teammsg', data: { text } });
    } else if (messageMode === 'all') {
        sendMessage({ type: 'message', data: { text } });
    } else if (messageMode.startsWith('private:')) {
        const targetId = parseInt(messageMode.split(':')[1]);
        sendMessage({ type: 'privmsg', data: { text, target: targetId } });
    }
    
    hideMessageInput();
}

function addMessage(text, type = '', fromPlayer = null, teamId = null) {
    const messages = document.getElementById('messages');
    const div = document.createElement('div');
    div.className = `message ${type}`;
    div.textContent = `[${new Date().toLocaleTimeString()}] ${text}`;
    
    // Set color based on team or player
    let color = '#888'; // Default gray for server messages
    
    if (fromPlayer !== null && gameState.players && gameState.players[fromPlayer]) {
        // Use the sender's team color
        const player = gameState.players[fromPlayer];
        color = teamColors[player.team] || '#888';
    } else if (teamId !== null) {
        // Use explicit team color if provided
        color = teamColors[teamId] || '#888';
    } else if (type === 'warning' || type === 'error') {
        // Warning/error messages in red
        color = '#f88';
    } else if (type === 'info') {
        // Info messages stay gray
        color = '#888';
    }
    
    div.style.color = color;
    messages.appendChild(div);
    messages.scrollTop = messages.scrollHeight;
    
    // Remove old messages
    while (messages.children.length > 100) {
        messages.removeChild(messages.firstChild);
    }
}

// Helper functions
function getTeamName(team) {
    switch(team) {
        case 1: return 'Fed';
        case 2: return 'Rom';
        case 4: return 'Kli';
        case 8: return 'Ori';
        default: return 'Ind';
    }
}

function getMaxShields(shipType) {
    const shields = [75, 85, 100, 130, 80, 500, 140];
    return shields[shipType] || 100;
}

function getMaxDamage(shipType) {
    const damage = [75, 85, 100, 130, 200, 600, 120];
    return damage[shipType] || 100;
}

// Bot control functions for practice mode

function balanceTeams() {
    if (!ws || ws.readyState !== WebSocket.OPEN) {
        // Not connected to server
        return;
    }
    
    sendMessage({ 
        type: 'message', 
        data: { 
            text: '/balance',
            to: 'all'
        } 
    });
}

function clearBots() {
    if (!ws || ws.readyState !== WebSocket.OPEN) {
        // Not connected to server
        return;
    }
    
    sendMessage({ 
        type: 'message', 
        data: { 
            text: '/clearbots',
            to: 'all'
        } 
    });
    
    // Clearing all bots
}

function fillWithBots() {
    if (!ws || ws.readyState !== WebSocket.OPEN) {
        // Not connected to server
        return;
    }
    
    // Count how many empty slots are available
    let freeSlots = 0;
    for (let i = 0; i < gameState.players.length; i++) {
        const player = gameState.players[i];
        if (!player || player.status === 0) { // Free slot
            freeSlots++;
        }
    }
    
    if (freeSlots === 0) {
        console.log('No free slots available');
        return;
    }
    
    // Send command to fill all slots with bots
    sendMessage({ 
        type: 'message', 
        data: { 
            text: '/fillbots',
            to: 'all'
        } 
    });
    
    console.log(`Filling ${freeSlots} slots with bots`);
}

function getMaxFuel(shipType) {
    const fuel = [5000, 7000, 10000, 14000, 6000, 60000, 12000];
    return fuel[shipType] || 10000;
}

function getMaxSpeed(shipType) {
    const speeds = [12, 10, 9, 8, 8, 2, 9]; // Scout, DD, CA, BB, AS, SB, GA
    return speeds[shipType] || 10;
}

function getMaxArmies(shipType) {
    const armies = [2, 5, 10, 6, 20, 25, 5]; // Scout, DD, CA, BB, AS, SB, GA
    return armies[shipType] || 10;
}