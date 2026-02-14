// Netrek Web Client

// Canonical team colors and letters - shared across all JS files via window globals
window.TEAM_COLORS = {
    0: '#888888',  // Ind - gray
    1: '#ffff00',  // Fed - yellow
    2: '#ff0000',  // Rom - red
    4: '#00ff00',  // Kli - green
    8: '#00ffff'   // Ori - cyan
};

window.TEAM_LETTERS = {
    0: 'I',  // Ind
    1: 'F',  // Fed
    2: 'R',  // Rom
    4: 'K',  // Kli
    8: 'O'   // Ori
};

// Local alias for use throughout this file
const teamColors = window.TEAM_COLORS;

// Visual constants for galactic map
const GALACTIC_DIM_ALPHA = 0.5;        // Alpha level for dimmed ships
const GALACTIC_NEUTRAL_GRAY = '#888';  // Neutral gray for cloaked enemies

// Tactical view scale: galaxy units to screen pixels (40 units per pixel, 20000 units visible)
const TACTICAL_SCALE = 0.025;

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
    
    // Update the display using cached refs
    if (dashboardEls.fedPlanets) dashboardEls.fedPlanets.textContent = counts[1];
    if (dashboardEls.romPlanets) dashboardEls.romPlanets.textContent = counts[2];
    if (dashboardEls.kliPlanets) dashboardEls.kliPlanets.textContent = counts[4];
    if (dashboardEls.oriPlanets) dashboardEls.oriPlanets.textContent = counts[8];
    if (dashboardEls.indPlanets) dashboardEls.indPlanets.textContent = counts[0];
}

// Cached DOM references for updateTeamDisplay (avoids repeated queries)
let _teamDisplayCache = null;
function getTeamDisplayElements() {
    if (!_teamDisplayCache) {
        _teamDisplayCache = {
            total: document.getElementById('totalPlayers'),
            counts: [
                document.getElementById('fedCount'),
                document.getElementById('romCount'),
                document.getElementById('kliCount'),
                document.getElementById('oriCount')
            ],
            labels: [
                document.querySelector('label[for="teamFed"]'),
                document.querySelector('label[for="teamRom"]'),
                document.querySelector('label[for="teamKli"]'),
                document.querySelector('label[for="teamOri"]')
            ],
            radios: [
                document.getElementById('teamFed'),
                document.getElementById('teamRom'),
                document.getElementById('teamKli'),
                document.getElementById('teamOri')
            ]
        };
    }
    return _teamDisplayCache;
}

// Update team display with the given data
function updateTeamDisplay(data) {
            const elems = getTeamDisplayElements();

            // Update total players display
            if (elems.total) {
                elems.total.textContent = `${data.total} player${data.total !== 1 ? 's' : ''} online`;
            }

            // Update team radio button labels with player counts
            const countTexts = [data.teams.fed, data.teams.rom, data.teams.kli, data.teams.ori];
            for (let i = 0; i < elems.counts.length; i++) {
                if (elems.counts[i]) elems.counts[i].textContent = `(${countTexts[i]})`;
            }

            // Highlight teams with fewer players for balance
            const counts = countTexts;
            const minCount = Math.min(...counts);
            const maxCount = Math.max(...counts);

            const teamLabels = elems.labels;
            const teamRadios = elems.radios;
            
            let needNewSelection = false;
            let firstAvailableIndex = -1;
            
            for (let i = 0; i < teamLabels.length; i++) {
                if (!teamLabels[i] || !teamRadios[i]) continue;
                const count = counts[i];
                // Remove any existing star indicator
                if (teamLabels[i].dataset.hasStar) {
                    teamLabels[i].textContent = teamLabels[i].dataset.originalText || teamLabels[i].textContent;
                    delete teamLabels[i].dataset.hasStar;
                }
                
                if (count === maxCount && maxCount > minCount + 1) {
                    // This team has significantly more players - disable it
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
                        teamLabels[i].dataset.originalText = teamLabels[i].textContent;
                        teamLabels[i].dataset.hasStar = '1';
                        teamLabels[i].textContent += ' \u2B50';
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
    fetch('/api/teams')
        .then(response => {
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            return response.json();
        })
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
    updateInterval: 0,
    interpolation: true,
    quitRequested: false // Track if player has requested to quit
};

// Victory countdown state
let victoryCountdown = 0;      // current seconds remaining
let victoryTimerId = null;     // interval handle

// Store previous positions for interpolation
let prevState = {
    players: [],
    torps: [],
    plasmas: []
};

let controls = {
    mouseX: 0,
    mouseY: 0
};

let canvases = {
    tactical: null,
    galactic: null,
    tacticalCtx: null,
    galacticCtx: null
};

// Cached DOM element references for dashboard (populated during init)
let dashboardEls = {};

// Centroid mode for team markers on galactic map: 'none', 'average', 'median'
let centroidMode = 'none';

// Ship names
const shipNames = ['SC', 'DD', 'CA', 'BB', 'AS', 'SB'];

// Performance tracking
let fps = 0;
let frameCount = 0;
let lastRenderTime = 0;
let lastFpsUpdate = 0;

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

// Track render loop and initialization state to prevent accumulation on reconnect
let renderIntervalId = null;
let gameInitialized = false;
let inputHandlersRegistered = false; // Prevent duplicate event listener registration
let savedCredentials = null; // { name, team, ship } saved on first connect for reconnect
let reconnectDelay = 1000; // Exponential backoff delay for reconnection
let reconnectAttempts = 0;
const maxReconnectAttempts = 10;

// Victory countdown functions
let victoryCountdownCompleted = false;

function startVictoryCountdown() {
    stopVictoryCountdown();
    victoryCountdownCompleted = false;
    victoryCountdown = 10;
    victoryTimerId = setInterval(() => {
        victoryCountdown--;
        if (victoryCountdown <= 0) {
            victoryCountdownCompleted = true;
            stopVictoryCountdown();
        }
    }, 1000);
}

function stopVictoryCountdown() {
    if (victoryTimerId !== null) {
        clearInterval(victoryTimerId);
        victoryTimerId = null;
    }
    victoryCountdown = 0;
}

function getVictoryCountdownMessage() {
    if (victoryCountdown > 0) {
        return `New game starting in ${victoryCountdown} second${victoryCountdown !== 1 ? 's' : ''}...`;
    }
    return 'New game starting...';
}

// Initialize the game (safe to call multiple times - only initializes once)
async function init() {
    if (gameInitialized) return;
    gameInitialized = true;

    // Set up canvases
    canvases.tactical = document.getElementById('tactical');
    canvases.galactic = document.getElementById('galactic-map');
    if (!canvases.tactical || !canvases.galactic) {
        console.error('Required canvas elements not found');
        gameInitialized = false;
        return;
    }
    canvases.tacticalCtx = canvases.tactical.getContext('2d');
    canvases.galacticCtx = canvases.galactic.getContext('2d');

    // Planet renderer is now simple circles - no initialization needed

    // Initialize ship renderer
    if (window.shipRenderer) {
        try {
            await window.shipRenderer.init();
            // Ship renderer initialized
        } catch (err) {
            console.warn('Failed to initialize ship renderer:', err);
            // Continue without ship bitmaps
        }
    }

    // Cache dashboard DOM references
    dashboardEls = {
        shields: document.getElementById('shields'),
        damage: document.getElementById('damage'),
        fuel: document.getElementById('fuel'),
        wtemp: document.getElementById('wtemp'),
        etemp: document.getElementById('etemp'),
        speed: document.getElementById('speed'),
        kdaStats: document.getElementById('kda-stats'),
        kdRatio: document.getElementById('kd-ratio'),
        updateInterval: document.getElementById('network-delay'),
        compression: document.getElementById('compression-indicator'),
        centroid: document.getElementById('centroid-indicator'),
        armies: document.getElementById('armies'),
        status: document.getElementById('status'),
        tournamentDisplay: document.getElementById('tournament-timer-display'),
        tournamentTimer: document.getElementById('tournament-timer-value'),
        alertStatus: document.getElementById('alert-status'),
        fedPlanets: document.getElementById('fed-planets'),
        romPlanets: document.getElementById('rom-planets'),
        kliPlanets: document.getElementById('kli-planets'),
        oriPlanets: document.getElementById('ori-planets'),
        indPlanets: document.getElementById('ind-planets'),
        // Cached refs for hot-path DOM lookups (keypress, every tick)
        messageInput: document.getElementById('message-input'),
        helpWindow: document.getElementById('help-window'),
        practicePanel: document.getElementById('practice-panel'),
        playerList: document.getElementById('player-list'),
    };

    // Resize canvases
    resizeCanvases();

    // Register event listeners only once to prevent accumulation on re-init
    if (!inputHandlersRegistered) {
        inputHandlersRegistered = true;

        let resizeTimer;
        window.addEventListener('resize', () => {
            clearTimeout(resizeTimer);
            resizeTimer = setTimeout(resizeCanvases, 100);
        });

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

        // Keyboard
        document.addEventListener('keydown', (e) => {
            // Prevent Firefox Quick Find when pressing / for slash commands
            if (e.key === '/' && !e.ctrlKey && !e.altKey && !e.metaKey) {
                e.preventDefault();
            }
            handleKeyPress(e.key);
        });
    }
    
    // Start render loop using requestAnimationFrame with 10 FPS target
    // This stops rendering in background tabs (saving CPU/battery) and syncs with vsync
    if (renderIntervalId !== null) {
        cancelAnimationFrame(renderIntervalId);
    }
    lastRenderTime = 0;
    function renderLoop(timestamp) {
        if (timestamp - lastRenderTime >= 95) { // ~10 FPS with slight tolerance
            lastRenderTime = timestamp;
            render();
        }
        renderIntervalId = requestAnimationFrame(renderLoop);
    }
    renderIntervalId = requestAnimationFrame(renderLoop);
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
                    console.warn('Error firing phaser:', err);
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
}

function handleKeyPress(key) {
    if (gameState.myPlayerID < 0) return;
    
    // Don't handle keys if typing a message
    if (dashboardEls.messageInput && dashboardEls.messageInput.style.display === 'block') {
        return;
    }

    // Handle help window toggle first (works even when dead)
    if (key === '?') {
        const helpWindow = dashboardEls.helpWindow;
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
        const helpWindow = dashboardEls.helpWindow;
        if (helpWindow && helpWindow.style.display === 'block') {
            helpWindow.style.display = 'none';
            windowClosed = true;
        }

        // Close info window if open
        if (window.infoWindow && window.infoWindow.destroy) {
            window.infoWindow.destroy();
            windowClosed = true;
        }

        // Close practice panel if open
        const practicePanel = dashboardEls.practicePanel;
        if (practicePanel && practicePanel.classList.contains('show')) {
            practicePanel.classList.remove('show');
            windowClosed = true;
        }
        
        if (windowClosed) {
            return;
        }
    }
    
    // Cycle team centroid mode (works even when dead)
    if (key === 'm') {
        const modes = ['none', 'average', 'median'];
        centroidMode = modes[(modes.indexOf(centroidMode) + 1) % modes.length];
        if (dashboardEls.centroid) {
            dashboardEls.centroid.textContent = centroidMode === 'none' ? 'OFF' : centroidMode.toUpperCase();
            dashboardEls.centroid.style.color = centroidMode === 'none' ? '#888' : '#0f0';
        }
        return;
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
    // Requires double-tap within 2 seconds to confirm (avoids blocking confirm() dialog)
    if (key === 'Q') {
        const now = Date.now();
        if (gameState._quitPending && now - gameState._quitPending < 2000) {
            delete gameState._quitPending;
            sendMessage({ type: 'quit', data: {} });
            gameState.quitRequested = true;
        } else {
            gameState._quitPending = now;
            addMessage('Press Q again within 2 seconds to self-destruct.', 'warning', null, null, 'messages-server');
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
            let nearestPressorDist = 6000;
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
            // Check if original key was uppercase T (Shift+T) for Team message
            if (key === 'T') {
                showMessageInput('team');
            } else {
                // Find nearest enemy for tractor beam
                let nearestEnemy = -1;
                let nearestDist = 6000;
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
                        // Mouse is on tactical map - use same scale as rendering
                        const scale = TACTICAL_SCALE;
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
    
    // Hide game interface, show login screen
    document.getElementById('game').style.display = 'none';
    document.getElementById('login').style.display = 'block';
    
    // Clear victory overlay state
    gameState.gameOver = false;
    stopVictoryCountdown();
    
    // Get current player to pre-select their team
    const myPlayer = gameState.players[gameState.myPlayerID];
    if (myPlayer && myPlayer.team) {
        // Pre-select the radio button for current team
        const teamValue = myPlayer.team;
        const teamRadios = document.querySelectorAll('input[name="team"]');
        teamRadios.forEach(r => { if (parseInt(r.value, 10) === teamValue) r.checked = true; });
        
        // Pre-select the radio button for current ship
        const shipValue = parseInt(myPlayer.ship, 10);
        const shipRadio = (Number.isFinite(shipValue) && shipValue >= 0 && shipValue <= 6)
            ? document.querySelector(`input[name="ship"][value="${shipValue}"]`)
            : null;
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
    
    const teamRadio = document.querySelector('input[name="team"]:checked');
    const shipRadio = document.querySelector('input[name="ship"]:checked');
    const team = teamRadio ? parseInt(teamRadio.value, 10) : 1;
    const ship = shipRadio ? parseInt(shipRadio.value, 10) : 0;
    
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

    const teamRadio = document.querySelector('input[name="team"]:checked');
    const shipRadio = document.querySelector('input[name="ship"]:checked');
    const team = teamRadio ? parseInt(teamRadio.value, 10) : 1;
    const ship = shipRadio ? parseInt(shipRadio.value, 10) : 0;

    // Save credentials for reconnect so we don't re-read hidden form elements
    savedCredentials = { name, team, ship };

    // Hide login, show game
    document.getElementById('login').style.display = 'none';
    document.getElementById('game').style.display = 'block';

    // Update compression indicator immediately
    updateCompressionIndicator();

    // Initialize game (with async handling) - init() is safe to call multiple times
    init().then(() => {
        // Game initialized successfully
    }).catch(err => {
        // Failed to initialize game
    });

    openWebSocket(name, team, ship);
}

// Reconnect using saved credentials (does not re-read form or re-init)
function reconnect() {
    if (!savedCredentials) return;
    if (uiState.inOutfitScreen) return; // Don't auto-rejoin while on team selection
    openWebSocket(savedCredentials.name, savedCredentials.team, savedCredentials.ship);
}

function openWebSocket(name, team, ship) {
    // Close existing connection if any
    if (ws) {
        ws.onclose = null; // Prevent triggering reconnect from this close
        if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
            ws.close();
        }
    }

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
        // Connected to server - reset backoff
        reconnectDelay = 1000;
        reconnectAttempts = 0;

        // Clear stale game state from previous session to prevent rendering
        // ghost players/projectiles between reconnect and first server update.
        gameState.players = [];
        gameState.planets = [];
        gameState.torps = [];
        gameState.plasmas = [];
        gameState.phasers = [];
        prevState.players = [];
        prevState.torps = [];
        prevState.plasmas = [];
        // Check if compression is enabled by examining the WebSocket extensions
        wsCompressionActive = !!(ws.extensions && ws.extensions.includes('permessage-deflate'));
        updateCompressionIndicator();

        sendMessage({
            type: 'login',
            data: { name: name, team: team, ship: ship }
        });
    };

    ws.onmessage = (event) => {
        try {
            const msg = JSON.parse(event.data);
            handleServerMessage(msg);
        } catch (e) {
            console.error('Failed to parse server message:', e);
        }
    };

    ws.onerror = (error) => {
        // WebSocket error
        addMessage('Connection error!', 'warning', null, null, 'messages-server');
    };

    // Capture the WebSocket instance for the closure to avoid stale reference
    const thisWs = ws;
    ws.onclose = () => {
        // Disconnected from server
        addMessage('Disconnected from server', 'warning', null, null, 'messages-server');
        // Only reconnect if this is still the current WebSocket and under retry limit
        if (reconnectAttempts >= maxReconnectAttempts) {
            addMessage('Reconnection failed after multiple attempts. Refresh to try again.', 'warning', null, null, 'messages-server');
            return;
        }
        const delay = reconnectDelay;
        reconnectDelay = Math.min(reconnectDelay * 2, 30000); // Exponential backoff, max 30s
        reconnectAttempts++;
        setTimeout(() => {
            if (ws === thisWs && savedCredentials && !gameState.quitRequested) {
                addMessage(`Attempting to reconnect (${reconnectAttempts}/${maxReconnectAttempts})...`, 'info', null, null, 'messages-server');
                reconnect();
            }
        }, delay);
    };
}

function sendMessage(msg) {
    if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify(msg));
    }
}

function handleServerMessage(msg) {
    if (!msg || !msg.type) return;
    if (msg.type === 'update' && (!msg.data || typeof msg.data !== 'object')) return;
    switch(msg.type) {
        case 'login_success':
            gameState.myPlayerID = msg.data.player_id;
            addMessage(`Joined as player ${msg.data.player_id}`, 'info', null, null, 'messages-server');
            break;
            
        case 'update':
            // Store previous positions for interpolation (only fields needed)
            // Copy previous positions in-place to avoid allocating new arrays/objects every tick
            for (let pi = 0; pi < gameState.players.length; pi++) {
                const p = gameState.players[pi];
                if (!prevState.players[pi]) prevState.players[pi] = {x: 0, y: 0, dir: 0, status: 0};
                if (p) { prevState.players[pi].x = p.x; prevState.players[pi].y = p.y; prevState.players[pi].dir = p.dir; prevState.players[pi].status = p.status; }
            }
            prevState.players.length = gameState.players.length;
            for (let ti = 0; ti < gameState.torps.length; ti++) {
                const t = gameState.torps[ti];
                if (!prevState.torps[ti]) prevState.torps[ti] = {x: 0, y: 0};
                if (t) { prevState.torps[ti].x = t.x; prevState.torps[ti].y = t.y; }
            }
            prevState.torps.length = gameState.torps.length;
            for (let qi = 0; qi < gameState.plasmas.length; qi++) {
                const q = gameState.plasmas[qi];
                if (!prevState.plasmas[qi]) prevState.plasmas[qi] = {x: 0, y: 0};
                if (q) { prevState.plasmas[qi].x = q.x; prevState.plasmas[qi].y = q.y; }
            }
            prevState.plasmas.length = gameState.plasmas.length;
            
            // Calculate network delay before updating lastUpdate
            const now = Date.now();
            if (gameState.lastUpdate) {
                gameState.updateInterval = now - gameState.lastUpdate;
            } else {
                gameState.updateInterval = 0;
            }
            
            gameState.frame = msg.data.frame;
            gameState.players = Array.isArray(msg.data.players) ? msg.data.players : [];
            gameState.planets = Array.isArray(msg.data.planets) ? msg.data.planets : [];
            gameState.torps = Array.isArray(msg.data.torps) ? msg.data.torps : [];
            gameState.plasmas = Array.isArray(msg.data.plasmas) ? msg.data.plasmas : [];
            gameState.gameOver = msg.data.gameOver || false;
            gameState.winner = msg.data.winner;
            gameState.winType = msg.data.winType;
            gameState.tMode = !!msg.data.tMode;
            gameState.tRemain = msg.data.tRemain;

            // Update planet counter
            updatePlanetCounter();
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
                    window.location.reload();
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
            // Handle player chat and server-generated messages
            const msgType = msg.data.type || '';
            const fromPlayer = msg.data.from !== undefined ? msg.data.from : null;
            const teamId = msg.data.team !== undefined ? msg.data.team : null;

            // Route messages based on type: player chat vs server events
            // Player-to-player chat messages (typed by users)
            const chatTypes = ['all', 'team', 'private'];
            const isPlayerChat = chatTypes.includes(msgType);
            const targetPanel = isPlayerChat ? 'messages-player' : 'messages-server';
            
            // Add message to appropriate panel
            addMessage(msg.data.text, msgType, fromPlayer, teamId, targetPanel);
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
            break;
            
        case 'error':
            addMessage(msg.data, 'warning', null, null, 'messages-server');
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
    const expectedInterval = 100; // 10 FPS = 100ms per frame
    const t = Math.min(timeSinceUpdate / expectedInterval, 1);
    
    // Find previous position
    const prev = Array.isArray(previous) ? previous[entityId] : previous;
    if (!prev) return current;

    // Skip interpolation when a player just respawned: the previous state
    // still holds the death-location coordinates, so lerping would briefly
    // flash the ship at its old position before it appears at the homeworld.
    if (prev.status !== undefined && current.status === 2 && prev.status !== 2) {
        return current;
    }

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
    
    // Handle victory countdown state transitions
    if (gameState.gameOver && victoryTimerId === null && !victoryCountdownCompleted) {
        startVictoryCountdown();
    } else if (!gameState.gameOver) {
        stopVictoryCountdown();
        victoryCountdownCompleted = false;
    }
    
    renderTactical();
    renderGalactic();
    // requestAnimationFrame loop continues in renderLoop()
}

function renderTactical() {
    // Decay phasers even when not rendering to prevent unbounded accumulation
    if (uiState.inOutfitScreen) {
        gameState.phasers = gameState.phasers.filter(p => { p.life--; return p.life > 0; });
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
        ctx.fillText(getVictoryCountdownMessage(), centerX, centerY + 60);
        
        ctx.restore();
        return; // Don't render game elements during victory screen
    }
    
    // Don't render if we don't have a valid player  
    if (gameState.myPlayerID < 0) {
        return;
    }
    
    // Get my player
    const myPlayer = gameState.players[gameState.myPlayerID];
    if (!myPlayer) {
        return;
    }
    
    const centerX = width / 2;
    const centerY = height / 2;
    const scale = TACTICAL_SCALE;
    
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
        
        // Check if we have info on this planet (for t-mode scouting)
        // Only apply scouting rules in tournament mode
        const hasInfo = gameState.tMode ? !!(planet.info & myPlayer.team) : true;
        
        // Use simplified planet renderer
        window.planetRenderer.drawTacticalPlanet(ctx, planet, screenX, screenY, hasInfo);
        
    }
    
    // Draw phaser beams (wrapped in save/restore to prevent globalAlpha leaks)
    gameState.phasers = gameState.phasers.filter(phaser => {
        if (phaser.life <= 0) return false;

        const fromPlayer = gameState.players[phaser.from];
        if (!fromPlayer) return false;

        ctx.save();
        
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
        
        // Parse hex color to RGB for rgba() gradient stops
        let r = 0, g = 0, b = 0;
        if (color.length === 4) {
            r = parseInt(color[1] + color[1], 16);
            g = parseInt(color[2] + color[2], 16);
            b = parseInt(color[3] + color[3], 16);
        } else if (color.length === 7) {
            r = parseInt(color.substring(1, 3), 16);
            g = parseInt(color.substring(3, 5), 16);
            b = parseInt(color.substring(5, 7), 16);
        }

        gradient.addColorStop(0, `rgba(${r},${g},${b},1)`);
        gradient.addColorStop(0.3, `rgba(${r},${g},${b},0.8)`);
        gradient.addColorStop(0.7, `rgba(${r},${g},${b},0.4)`);
        gradient.addColorStop(1, `rgba(${r},${g},${b},0)`);
        
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
        
        ctx.restore();
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
        
        // Allow margin for explosion effects (radius 12)
        if (screenX < -12 || screenX > width + 12 || screenY < -12 || screenY > height + 12) continue;

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
        
        // Show explosion effect when plasma hits something (status = 3)
        if (plasma.status === 3) {
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
            // Draw plasma as 8x8 square (looks like torpedo but bigger)
            const size = 8; // twice regular torpedo size
            ctx.fillStyle = teamColors[plasma.team] || '#888';
            ctx.fillRect(screenX - size / 2, screenY - size / 2, size, size);
        }
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

            ctx.save(); // Prevent globalAlpha leak from explosion rendering

            // Draw improved explosion with multiple effects
            const progress = Math.max(0, Math.min(1, 1 - (player.explodeTimer || 0) / 10));
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
            ctx.restore();
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
        
        // Reset context before drawing shields to ensure consistent alpha
        ctx.restore();
        
        // Shield circle if shields up - use completely fresh context
        if (player.shields_up) {
            // Store current state before creating fresh context
            ctx.save();
            
            // Reset all transformation and styling to defaults
            ctx.setTransform(1, 0, 0, 1, 0, 0);  // Reset transform matrix
            const shieldColor = teamColors[player.team] || '#fff';
            ctx.lineCap = 'butt';
            ctx.lineJoin = 'miter';
            ctx.setLineDash([]);

            // Outer glow layer
            ctx.globalAlpha = 0.15;
            ctx.strokeStyle = shieldColor;
            ctx.lineWidth = 4;
            ctx.beginPath();
            ctx.arc(screenX, screenY, 16, 0, Math.PI * 2);
            ctx.stroke();

            // Main shield circle
            ctx.globalAlpha = 0.55;
            ctx.strokeStyle = shieldColor;
            ctx.lineWidth = 1.5;
            ctx.beginPath();
            ctx.arc(screenX, screenY, 15, 0, Math.PI * 2);
            ctx.stroke();
            
            ctx.restore();
        }
        
        // Re-save context for repair indicator
        ctx.save();
        
        // Reapply cloaking alpha if needed for repair indicator
        if (player.cloaked && player.team === myPlayer.team) {
            ctx.globalAlpha = GALACTIC_DIM_ALPHA;
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

        // Player label below (team letter + slot number)
        const teamLetter = window.TEAM_LETTERS[player.team] || 'I';
        const playerLabel = teamLetter + i.toString().padStart(2, '0');
        
        ctx.fillStyle = teamColors[player.team] || '#fff';
        ctx.font = i === gameState.myPlayerID ? 'bold 9px monospace' : '9px monospace';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'top';
        ctx.fillText(playerLabel, screenX, screenY + 12);
    }

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
    
    // Get my player to check team for planet info
    const myPlayer = gameState.myPlayerID >= 0 ? gameState.players[gameState.myPlayerID] : null;
    const myTeam = myPlayer ? myPlayer.team : 1;
    
    // Draw planets
    for (const planet of gameState.planets) {
        if (!planet) continue;
        
        const x = planet.x * scale;
        const y = planet.y * scale;
        
        // Check if we have info on this planet
        // Only apply scouting rules in tournament mode
        const hasInfo = gameState.tMode ? !!(planet.info & myTeam) : true;
        
        // Use simplified planet renderer
        if (window.planetRenderer) {
            window.planetRenderer.drawGalacticPlanet(ctx, planet, x, y, hasInfo);
        } else {
            // Fallback to text labels
            ctx.font = '9px monospace';
            ctx.textAlign = 'center';
            ctx.textBaseline = 'middle';
            // Show actual color if we have info, otherwise gray
            ctx.fillStyle = hasInfo ? (teamColors[planet.owner] || '#888') : '#444';
            // Always show planet label
            const label = planet.label || (planet.name || '???').substring(0, 3).toUpperCase();
            ctx.fillText(label, x, y);
        }
    }
    
    // Draw players
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
        const teamLetter = window.TEAM_LETTERS[player.team] || 'I';
        const playerLabel = teamLetter + i.toString().padStart(2, '0');
        
        ctx.fillStyle = teamColors[player.team] || '#fff';
        ctx.font = i === gameState.myPlayerID ? 'bold 10px monospace' : '9px monospace';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText(playerLabel, x, y);
        
        ctx.restore();
    }
    
    // Draw team centroid markers
    if (centroidMode !== 'none') {
        const useMedian = centroidMode === 'median';
        const teams = [1, 2, 4, 8];
        for (const team of teams) {
            const color = teamColors[team] || '#fff';

            // Collect positions of alive players on this team
            const playerXs = [], playerYs = [];
            for (const player of gameState.players) {
                if (player && player.status === 2 && player.team === team) {
                    playerXs.push(player.x);
                    playerYs.push(player.y);
                }
            }

            let playerCX = null, playerCY = null;
            if (playerXs.length > 0) {
                if (useMedian) {
                    playerXs.sort((a, b) => a - b);
                    playerYs.sort((a, b) => a - b);
                    const mid = playerXs.length >> 1;
                    playerCX = (playerXs.length & 1 ? playerXs[mid] : (playerXs[mid - 1] + playerXs[mid]) / 2) * scale;
                    playerCY = (playerYs.length & 1 ? playerYs[mid] : (playerYs[mid - 1] + playerYs[mid]) / 2) * scale;
                } else {
                    playerCX = (playerXs.reduce((a, b) => a + b, 0) / playerXs.length) * scale;
                    playerCY = (playerYs.reduce((a, b) => a + b, 0) / playerYs.length) * scale;
                }
            }

            // Collect positions of planets owned by this team
            const planetXs = [], planetYs = [];
            for (const planet of gameState.planets) {
                if (planet && planet.owner === team) {
                    planetXs.push(planet.x);
                    planetYs.push(planet.y);
                }
            }

            let planetCX = null, planetCY = null;
            if (planetXs.length > 0) {
                if (useMedian) {
                    planetXs.sort((a, b) => a - b);
                    planetYs.sort((a, b) => a - b);
                    const mid = planetXs.length >> 1;
                    planetCX = (planetXs.length & 1 ? planetXs[mid] : (planetXs[mid - 1] + planetXs[mid]) / 2) * scale;
                    planetCY = (planetYs.length & 1 ? planetYs[mid] : (planetYs[mid - 1] + planetYs[mid]) / 2) * scale;
                } else {
                    planetCX = (planetXs.reduce((a, b) => a + b, 0) / planetXs.length) * scale;
                    planetCY = (planetYs.reduce((a, b) => a + b, 0) / planetYs.length) * scale;
                }
            }

            // Draw arrow from planet centroid to player centroid
            if (playerCX !== null && planetCX !== null) {
                const dx = playerCX - planetCX;
                const dy = playerCY - planetCY;
                const len = Math.sqrt(dx * dx + dy * dy);
                if (len > 1) {
                    ctx.save();
                    ctx.globalAlpha = 0.6;
                    ctx.strokeStyle = color;
                    ctx.lineWidth = 1.5;

                    // Shaft
                    ctx.beginPath();
                    ctx.moveTo(planetCX, planetCY);
                    ctx.lineTo(playerCX, playerCY);
                    ctx.stroke();

                    // Arrowhead
                    const headLen = 7;
                    const ux = dx / len;
                    const uy = dy / len;
                    ctx.beginPath();
                    ctx.moveTo(playerCX, playerCY);
                    ctx.lineTo(playerCX - headLen * ux + headLen * 0.5 * uy,
                               playerCY - headLen * uy - headLen * 0.5 * ux);
                    ctx.moveTo(playerCX, playerCY);
                    ctx.lineTo(playerCX - headLen * ux - headLen * 0.5 * uy,
                               playerCY - headLen * uy + headLen * 0.5 * ux);
                    ctx.stroke();

                    ctx.restore();
                }
            }
        }
    }

    // Lock indicator on galactic map removed
}

function updateCompressionIndicator() {
    const indicator = dashboardEls.compression || document.getElementById('compression-indicator');
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
    
    // Update tick interval (time between game state updates; ~100ms is normal at 10 FPS)
    const lag = gameState.updateInterval || 0;
    if (dashboardEls.updateInterval) {
        dashboardEls.updateInterval.textContent = `${lag}ms`;
        if (lag < 150) {
            dashboardEls.updateInterval.style.color = '#0f0';  // Green
        } else if (lag < 200) {
            dashboardEls.updateInterval.style.color = '#ff0';  // Yellow
        } else {
            dashboardEls.updateInterval.style.color = '#f00';  // Red
        }
    }

    // Update stats with current/max format where applicable
    const maxShields = getMaxShields(player.ship);
    const maxDamage = getMaxDamage(player.ship);
    const maxFuel = getMaxFuel(player.ship);
    const maxSpeed = getMaxSpeed(player.ship);
    const maxArmies = getMaxArmies(player.ship);

    if (dashboardEls.shields) dashboardEls.shields.textContent = `${player.shields || 0} / ${maxShields}`;
    if (dashboardEls.damage) dashboardEls.damage.textContent = `${player.damage || 0} / ${maxDamage}`;
    if (dashboardEls.fuel) dashboardEls.fuel.textContent = `${player.fuel || 0} / ${maxFuel}`;
    if (dashboardEls.wtemp) dashboardEls.wtemp.textContent = player.wtemp || 0;
    if (dashboardEls.etemp) dashboardEls.etemp.textContent = player.etemp || 0;
    if (dashboardEls.speed) dashboardEls.speed.textContent = `${Math.round(player.speed || 0)} / ${maxSpeed}`;

    // Update KS/K/D stats
    const killStreak = Math.floor(player.killsStreak || 0);
    const kills = Math.floor(player.kills || 0);
    const deaths = player.deaths || 0;
    if (dashboardEls.kdaStats) {
        dashboardEls.kdaStats.textContent = `${killStreak} / ${kills} / ${deaths}`;
        // Color based on kill streak
        if (killStreak >= 5) {
            dashboardEls.kdaStats.style.color = '#ff0'; // Yellow for high streak
        } else if (killStreak >= 3) {
            dashboardEls.kdaStats.style.color = '#0ff'; // Cyan for medium streak
        } else {
            dashboardEls.kdaStats.style.color = '#8f8'; // Light green default
        }
    }

    // Update K/D ratio
    if (dashboardEls.kdRatio) {
        const kdRatio = deaths > 0 ? (kills / deaths).toFixed(2) : kills.toFixed(2);
        dashboardEls.kdRatio.textContent = kdRatio;
        // Color based on K/D ratio
        if (parseFloat(kdRatio) >= 2.0) {
            dashboardEls.kdRatio.style.color = '#ff0'; // Yellow for excellent
        } else if (parseFloat(kdRatio) >= 1.0) {
            dashboardEls.kdRatio.style.color = '#0ff'; // Cyan for positive
        } else if (parseFloat(kdRatio) >= 0.5) {
            dashboardEls.kdRatio.style.color = '#8f8'; // Light green for okay
        } else {
            dashboardEls.kdRatio.style.color = '#f88'; // Light red for poor
        }
    }
    
    
    
    // Update armies and orbit status
    if (dashboardEls.armies) {
        dashboardEls.armies.textContent = `${player.armies || 0} / ${maxArmies}`;
        // Gray out armies section if kill streak is less than 2
        if (killStreak < 2) {
            dashboardEls.armies.style.color = '#888'; // Gray
        } else {
            dashboardEls.armies.style.color = '#0f0'; // Green (normal)
        }
    }

    if (dashboardEls.status) {
        dashboardEls.status.style.color = ''; // Reset color to default
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
            dashboardEls.status.style.color = '#f00'; // Make it red
        } else if (player.shields_up === false) {
            statusText = 'Shields Down';
        } else if (player.cloaked) {
            statusText = 'Cloaked';
        }
        dashboardEls.status.textContent = statusText;
    }

    // Update tournament mode display in planet counter
    if (dashboardEls.tournamentDisplay && dashboardEls.tournamentTimer) {
        if (gameState.tMode && !gameState.gameOver) {
            dashboardEls.tournamentDisplay.style.display = 'inline-block';
            const minutes = Math.floor(gameState.tRemain / 60);
            const seconds = gameState.tRemain % 60;
            dashboardEls.tournamentTimer.textContent = `${minutes}:${seconds.toString().padStart(2, '0')}`;
        } else {
            dashboardEls.tournamentDisplay.style.display = 'none';
        }
    }
    
    
    // Update alert status display
    updateAlertStatus();
}

function updateAlertStatus() {
    const alertEl = dashboardEls.alertStatus;
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

let lastPlayerListSignature = '';

function updatePlayerList() {
    const list = dashboardEls.playerList;
    if (!list) return;

    // Build a signature to skip DOM rebuild if nothing changed
    let sig = '';
    for (let i = 0; i < gameState.players.length; i++) {
        const p = gameState.players[i];
        if (p && p.status !== 0 && p.status !== 1) {
            sig += `${i}:${p.team}:${p.status}:${p.ship}:${p.name}:${Math.floor(p.killsStreak||0)}:${Math.floor(p.kills||0)}:${p.deaths||0};`;
        }
    }
    if (sig === lastPlayerListSignature) return;
    lastPlayerListSignature = sig;

    // Build header using DOM APIs
    const header = document.createElement('div');
    header.style.cssText = 'border-bottom: 1px solid #808080; margin-bottom: 5px; display: flex; justify-content: space-between; font-size: 9px; color: #c0c0c0;';
    const headerLeft = document.createElement('span');
    const headerIdLabel = document.createElement('span');
    headerIdLabel.style.fontFamily = 'monospace';
    headerIdLabel.style.marginRight = '4px';
    headerIdLabel.textContent = 'ID ';
    headerLeft.appendChild(headerIdLabel);
    headerLeft.appendChild(document.createTextNode('\u00a0PLAYERS'));
    const headerRight = document.createElement('span');
    headerRight.textContent = 'KS/K/D/KD';
    header.appendChild(headerLeft);
    header.appendChild(headerRight);

    const fragment = document.createDocumentFragment();
    fragment.appendChild(header);
    
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
        const teamLetter = window.TEAM_LETTERS[player.team] || 'I';
        const playerID = teamLetter + player.slot.toString().padStart(2, '0');
        
        // Add opacity style for dead players
        const deadStyle = isDead ? 'opacity: 0.4;' : '';
        
        const entry = document.createElement('div');
        entry.className = `player-entry ${teamClass}`;
        entry.style.cssText = `display: flex; justify-content: space-between; ${deadStyle}`;

        const nameSpan = document.createElement('span');
        const idSpan = document.createElement('span');
        idSpan.style.fontFamily = 'monospace';
        idSpan.style.marginRight = '4px';
        idSpan.textContent = playerID;
        nameSpan.appendChild(idSpan);
        nameSpan.appendChild(document.createTextNode(` ${player.name || 'Player'} (${shipType})`));

        const statsSpan = document.createElement('span');
        statsSpan.style.fontSize = '9px';
        statsSpan.textContent = `${Math.floor(killsStreak)} / ${Math.floor(kills)} / ${deaths} / ${kd}`;

        entry.appendChild(nameSpan);
        entry.appendChild(statsSpan);
        fragment.appendChild(entry);
    }

    list.replaceChildren(fragment);
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
        const targetId = parseInt(mode.split(':')[1], 10);
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
    let closestPlayerIndex = -1;

    // Get mouse position on the active canvas
    let mouseX, mouseY;
    if (controls.activeCanvas === 'tactical') {
        mouseX = controls.mouseX;
        mouseY = controls.mouseY;
        
        const canvas = canvases.tactical;
        const centerX = canvas.width / 2;
        const centerY = canvas.height / 2;
        const scale = TACTICAL_SCALE;
        
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
            
            const dist = Math.sqrt(Math.pow(screenX - mouseX, 2) + Math.pow(screenY - mouseY, 2));
            if (dist < closestDistance) {
                closestDistance = dist;
                closestTarget = player;
                targetType = 'player';
                closestPlayerIndex = i;
            }
        }

        // Check planets
        for (const planet of gameState.planets) {
            if (!planet) continue;
            
            const dx = (planet.x - myPlayer.x) * scale;
            const dy = (planet.y - myPlayer.y) * scale;
            const screenX = centerX + dx;
            const screenY = centerY + dy;
            
            const dist = Math.sqrt(Math.pow(screenX - mouseX, 2) + Math.pow(screenY - mouseY, 2));
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
            
            const dist = Math.sqrt(Math.pow(screenX - mouseX, 2) + Math.pow(screenY - mouseY, 2));
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
            
            const dist = Math.sqrt(Math.pow(screenX - mouseX, 2) + Math.pow(screenY - mouseY, 2));
            if (dist < closestDistance) {
                closestDistance = dist;
                closestTarget = player;
                targetType = 'player';
                closestPlayerIndex = i;
            }
        }
    }

    // Show info window if we found something close enough
    if (closestTarget && closestDistance < 100) { // Within 100 pixels
        if (window.infoWindow) {
            // Get actual screen coordinates for window placement
            const rect = controls.activeCanvas === 'tactical' ? 
                canvases.tactical.getBoundingClientRect() : 
                canvases.galactic.getBoundingClientRect();
            const windowX = rect.left + mouseX + 20;
            const windowY = rect.top + mouseY;
            
            if (targetType === 'planet') {
                window.infoWindow.showPlanetInfo(closestTarget, windowX, windowY);
            } else if (targetType === 'player') {
                window.infoWindow.showPlayerInfo(closestTarget, windowX, windowY, closestPlayerIndex);
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
        const targetId = parseInt(messageMode.split(':')[1], 10);
        if (!isNaN(targetId)) {
            sendMessage({ type: 'privmsg', data: { text, target: targetId } });
        }
    }
    
    hideMessageInput();
}

function addMessage(text, type = '', fromPlayer = null, teamId = null, targetPanel = null) {
    // Determine which panel to use for message routing
    let panelId = 'messages-server'; // Default: all unknown messages go to SERVER MESSAGES panel

    if (targetPanel) {
        // Explicit panel was specified by caller - use it directly
        panelId = targetPanel;
    } else {
        // Fallback logic for backward compatibility (this should rarely be used now)
        // Note: The new routing logic in handleServerMessage should always specify targetPanel
        const playerChatTypes = ['all', 'team', 'private', 'privmsg'];
        if (playerChatTypes.includes(type)) {
            panelId = 'messages-player';
        }
        // All other types (kill, info, warning, error, etc.) default to messages-server
    }

    const messages = document.getElementById(panelId);
    if (!messages) {
        // Fallback to old single messages div if it exists
        const fallback = document.getElementById('messages');
        if (fallback) {
            const div = document.createElement('div');
            const VALID_FALLBACK_TYPES = ['all', 'team', 'private', 'kill', 'warning', 'error', 'info', 'victory'];
            const safeFallbackType = VALID_FALLBACK_TYPES.includes(type) ? type : '';
            div.className = `message ${safeFallbackType}`;
            div.textContent = `[${new Date().toLocaleTimeString()}] ${text}`;
            // Apply same color logic as main path
            let fallbackColor = '#888';
            if (fromPlayer !== null && gameState.players && gameState.players[fromPlayer]) {
                fallbackColor = teamColors[gameState.players[fromPlayer].team] || fallbackColor;
            } else if (teamId !== null) {
                fallbackColor = teamColors[teamId] || fallbackColor;
            } else if (type === 'warning' || type === 'error') {
                fallbackColor = '#f88';
            }
            div.style.color = fallbackColor;
            fallback.appendChild(div);
            fallback.scrollTop = fallback.scrollHeight;
        }
        return;
    }

    const div = document.createElement('div');
    const VALID_MSG_TYPES = ['all', 'team', 'private', 'kill', 'warning', 'error', 'info', 'victory'];
    const safeType = VALID_MSG_TYPES.includes(type) ? type : '';
    div.className = `message ${safeType}`;
    div.textContent = `[${new Date().toLocaleTimeString()}] ${text}`;

    // Set color based on team or player
    let color = '#888'; // Default gray for server messages

    if (fromPlayer !== null && gameState.players && gameState.players[fromPlayer]) {
        // Use the sender's team color
        const player = gameState.players[fromPlayer];
        color = teamColors[player.team] || color;
    } else if (teamId !== null) {
        // Use explicit team color if provided
        color = teamColors[teamId] || color;
    } else if (type === 'warning' || type === 'error') {
        // Warning/error messages in red
        color = '#f88';
    }
    // Info messages use team color if available, otherwise stay default gray

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

// Ship stats lookup table indexed by ship type (0=SC, 1=DD, 2=CA, 3=BB, 4=AS, 5=SB)
// Must match server-side game.ShipData values
const SHIP_STATS = [
    { shields: 75,  damage: 75,  fuel: 5000,  speed: 12, armies: 2  }, // Scout
    { shields: 85,  damage: 85,  fuel: 7000,  speed: 10, armies: 5  }, // Destroyer
    { shields: 100, damage: 100, fuel: 10000, speed: 9,  armies: 10 }, // Cruiser
    { shields: 130, damage: 130, fuel: 14000, speed: 8,  armies: 6  }, // Battleship
    { shields: 80,  damage: 200, fuel: 6000,  speed: 8,  armies: 20 }, // Assault
    { shields: 500, damage: 600, fuel: 60000, speed: 2,  armies: 25 }, // Starbase
];

function getMaxShields(shipType) {
    return (SHIP_STATS[shipType] && SHIP_STATS[shipType].shields) || 100;
}

function getMaxDamage(shipType) {
    return (SHIP_STATS[shipType] && SHIP_STATS[shipType].damage) || 100;
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
}

function getMaxFuel(shipType) {
    return (SHIP_STATS[shipType] && SHIP_STATS[shipType].fuel) || 10000;
}

function getMaxSpeed(shipType) {
    return (SHIP_STATS[shipType] && SHIP_STATS[shipType].speed) || 10;
}

function getMaxArmies(shipType) {
    return (SHIP_STATS[shipType] && SHIP_STATS[shipType].armies) || 10;
}
