// Planet/Player Info Window - Based on original Netrek client inform.c

// Escape HTML special characters to prevent XSS
function escapeHtml(str) {
    if (str == null) return '';
    return String(str)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#039;');
}

// Helper function to format team letter and slot number (F01 format)
function formatTeamSlot(player, playerIndex) {
    const teamMap = {0: 'I', 1: 'F', 2: 'R', 4: 'K', 8: 'O'};
    const letter = teamMap[player.team] || 'I';
    const slot = String(playerIndex).padStart(2, '0');
    return letter + slot;           // e.g. F01
}

class InfoWindow {
    constructor() {
        this.visible = false;
        this.element = null;
        this.timeout = null;
        this.keepInfoTime = 15; // seconds to keep window open
        this.currentTarget = null; // Store current target
        this.targetType = null; // 'planet' or 'player'
        this.windowX = 0;
        this.windowY = 0;
        this.playerIndex = -1; // Store player index for team/slot display
    }
    
    // Create the info window element
    createWindow(x, y) {
        // Remove existing window if any
        this.destroy();
        
        // Create new window
        this.element = document.createElement('div');
        this.element.id = 'info-window';
        this.element.style.cssText = `
            position: absolute;
            left: ${x}px;
            top: ${y}px;
            background: rgba(0, 0, 0, 0.95);
            border: 2px solid #ccc;
            padding: 8px;
            font-family: 'Courier New', monospace;
            font-size: 12px;
            color: #ccc;
            z-index: 1000;
            min-width: 250px;
            box-shadow: 0 0 10px rgba(200, 200, 200, 0.3);
        `;
        
        // Adjust position if too close to any edge
        const maxX = window.innerWidth - 300;
        const maxY = window.innerHeight - 200;
        if (x > maxX) this.element.style.left = Math.max(0, maxX) + 'px';
        if (y > maxY) this.element.style.top = Math.max(0, maxY) + 'px';
        if (x < 0) this.element.style.left = '0px';
        if (y < 0) this.element.style.top = '0px';
        
        document.body.appendChild(this.element);
        this.visible = true;
        
        // Auto-close after timeout
        if (this.timeout) clearTimeout(this.timeout);
        this.timeout = setTimeout(() => this.destroy(), this.keepInfoTime * 1000);
    }
    
    // Show planet information
    showPlanetInfo(planet, x, y) {
        this.createWindow(x, y);
        
        // Store target info
        this.currentTarget = planet;
        this.targetType = 'planet';
        this.windowX = x;
        this.windowY = y;
        
        let html = '';
        
        // Get my team
        const myPlayer = gameState.players[gameState.myPlayerID];
        const myTeam = myPlayer ? myPlayer.team : 1;
        
        // Check if we have info on this planet (scouted by our team)
        // Only apply scouting rules in tournament mode
        if (!gameState.tMode || (planet.info && (planet.info & myTeam))) {
            // Planet name and owner
            const ownerName = this.getTeamName(planet.owner);
            html += `<div style="color: ${this.getTeamColor(planet.owner)}; font-weight: bold;">`;
            html += `${escapeHtml(planet.name)} (${escapeHtml(ownerName)})`;
            html += '</div>';

            // Army count
            html += `<div style="margin-top: 4px;">Armies: ${escapeHtml(planet.armies || 0)}</div>`;

            // Resources and info
            let resources = [];
            if (planet.flags & 16) resources.push('REPAIR');
            if (planet.flags & 32) resources.push('FUEL');
            if (planet.flags & 64) resources.push('AGRI');
            if (planet.flags & 2048) resources.push('CORE');

            // Who has info on this planet
            let info = [];
            if (planet.info & 1) info.push('F');
            if (planet.info & 2) info.push('R');
            if (planet.info & 4) info.push('K');
            if (planet.info & 8) info.push('O');

            html += '<div style="margin-top: 4px;">';
            if (resources.length > 0) {
                html += resources.join(' ') + ' ';
            } else {
                html += '(no resources) ';
            }
            html += info.join('');
            html += '</div>';
        } else {
            // No info on this planet - show name but indicate it's unscouted
            html += `<div style="color: #888; font-weight: bold;">${escapeHtml(planet.name)} (Unscouted)</div>`;
            html += '<div style="margin-top: 4px; color: #666;">Orbit to reveal owner and armies</div>';
        }

        this.element.innerHTML = html;
    }
    
    // Show player information
    showPlayerInfo(player, x, y, playerIndex = -1) {
        this.createWindow(x, y);
        
        // Store target info
        this.currentTarget = player;
        this.targetType = 'player';
        this.windowX = x;
        this.windowY = y;
        this.playerIndex = playerIndex; // Store the player index for updates
        
        const shipTypes = ['SC', 'DD', 'CA', 'BB', 'AS', 'SB', 'GA'];
        const shipName = shipTypes[player.ship] || '??';
        
        let html = '';
        
        // Player name, rank, ship type with team/slot identifier
        html += `<div style="color: ${this.getTeamColor(player.team)}; font-weight: bold;">`;
        if (playerIndex >= 0) {
            const teamSlot = formatTeamSlot(player, playerIndex);
            html += `<span style="font-weight: bold; margin-right: 8px; color: ${this.getTeamColor(player.team)};">${teamSlot}</span>`;
        }
        html += `${escapeHtml(player.name)} (${escapeHtml(player.rank) || 'Ensign'})`;
        html += '</div>';
        const kd = player.deaths > 0 ? (player.kills / player.deaths).toFixed(2) : Math.floor(player.kills).toFixed(1);
        html += `<div>${escapeHtml(shipName)} (${escapeHtml(Math.floor(player.killsStreak || 0))} / ${escapeHtml(Math.floor(player.kills))} / ${escapeHtml(player.deaths || 0)} / ${escapeHtml(kd)})</div>`;

        // Show detailed stats for all players (teammates, enemies, and self)
        // Stats
        html += '<div style="margin-top: 4px;">';
        html += `Speed: ${escapeHtml(Math.round(player.speed))} `;
        html += `Dam: ${escapeHtml(player.damage)}% `;
        html += `Sh: ${escapeHtml(player.shields)}% `;
        html += `Fuel: ${escapeHtml(player.fuel)}`;
        html += '</div>';

        // Status flags
        let status = [];
        if (player.shields_up) status.push('Shields');
        if (player.cloaked) status.push('Cloak');
        if (player.wtemp > 50) status.push('W-Temp');
        if (player.etemp > 50) status.push('E-Temp');
        if (player.armies > 0) status.push(`${escapeHtml(player.armies)} armies`);

        if (status.length > 0) {
            html += `<div style="margin-top: 4px;">${status.join(', ')}</div>`;
        }

        this.element.innerHTML = html;
    }
    
    // Get team color
    getTeamColor(team) {
        const colors = {
            1: '#ff0',  // Fed
            2: '#f00',  // Rom
            4: '#0f0',  // Kli
            8: '#0ff',  // Ori
            0: '#888'   // Nobody
        };
        return colors[team] || '#fff';
    }
    
    // Get team name
    getTeamName(team) {
        const names = {
            1: 'F',  // Fed
            2: 'R',  // Rom
            4: 'K',  // Kli
            8: 'O',  // Ori
            0: 'I'   // Independent
        };
        return names[team] || '?';
    }
    
    // Update the info window with current game state
    update() {
        if (!this.visible || !this.currentTarget || !this.element) return;
        
        // Find the updated target in the game state
        let updatedTarget = null;
        
        if (this.targetType === 'planet') {
            // Find planet by ID
            updatedTarget = gameState.planets.find(p => p && p.id === this.currentTarget.id);
        } else if (this.targetType === 'player') {
            // Find player by ID
            updatedTarget = gameState.players.find(p => p && p.id === this.currentTarget.id);
        }
        
        if (!updatedTarget) {
            // Target no longer exists, close window
            this.destroy();
            return;
        }
        
        // Check if an enemy player has cloaked - if so, close the window
        if (this.targetType === 'player') {
            const myPlayer = gameState.players[gameState.myPlayerID];
            if (myPlayer && updatedTarget.cloaked && updatedTarget.team !== myPlayer.team) {
                // Enemy has cloaked, close the window
                this.destroy();
                return;
            }
        }
        
        // Update the stored target
        this.currentTarget = updatedTarget;
        
        // Rebuild the content
        let html = '';
        
        if (this.targetType === 'planet') {
            // Get my team
            const myPlayer = gameState.players[gameState.myPlayerID];
            const myTeam = myPlayer ? myPlayer.team : 1;
            
            // Check if we have info on this planet (scouted by our team)
            // Only apply scouting rules in tournament mode
            if (!gameState.tMode || (updatedTarget.info && (updatedTarget.info & myTeam))) {
                // Planet name and owner
                const ownerName = this.getTeamName(updatedTarget.owner);
                html += `<div style="color: ${this.getTeamColor(updatedTarget.owner)}; font-weight: bold;">`;
                html += `${escapeHtml(updatedTarget.name)} (${escapeHtml(ownerName)})`;
                html += '</div>';

                // Army count
                html += `<div style="margin-top: 4px;">Armies: ${updatedTarget.armies || 0}</div>`;

                // Resources and info
                let resources = [];
                if (updatedTarget.flags & 16) resources.push('REPAIR');
                if (updatedTarget.flags & 32) resources.push('FUEL');
                if (updatedTarget.flags & 64) resources.push('AGRI');
                if (updatedTarget.flags & 2048) resources.push('CORE');

                // Who has info on this planet
                let info = [];
                if (updatedTarget.info & 1) info.push('F');
                if (updatedTarget.info & 2) info.push('R');
                if (updatedTarget.info & 4) info.push('K');
                if (updatedTarget.info & 8) info.push('O');

                html += '<div style="margin-top: 4px;">';
                if (resources.length > 0) {
                    html += resources.join(' ') + ' ';
                } else {
                    html += '(no resources) ';
                }
                html += info.join('');
                html += '</div>';
            } else {
                // No info on this planet - show name but indicate it's unscouted
                html += `<div style="color: #888; font-weight: bold;">${escapeHtml(updatedTarget.name)} (Unscouted)</div>`;
                html += '<div style="margin-top: 4px; color: #666;">Orbit to reveal owner and armies</div>';
            }
        } else if (this.targetType === 'player') {
            const shipTypes = ['SC', 'DD', 'CA', 'BB', 'AS', 'SB', 'GA'];
            const shipName = shipTypes[updatedTarget.ship] || '??';
            
            // Player name, rank, ship type with team/slot identifier
            html += `<div style="color: ${this.getTeamColor(updatedTarget.team)}; font-weight: bold;">`;
            if (this.playerIndex >= 0) {
                const teamSlot = formatTeamSlot(updatedTarget, this.playerIndex);
                html += `<span style="font-weight: bold; margin-right: 8px; color: ${this.getTeamColor(updatedTarget.team)};">${teamSlot}</span>`;
            }
            html += `${escapeHtml(updatedTarget.name)} (${escapeHtml(updatedTarget.rank) || 'Ensign'})`;
            html += '</div>';
            const kd = updatedTarget.deaths > 0 ? (updatedTarget.kills / updatedTarget.deaths).toFixed(2) : Math.floor(updatedTarget.kills).toFixed(1);
            html += `<div>${shipName} (${Math.floor(updatedTarget.killsStreak || 0)} / ${Math.floor(updatedTarget.kills)} / ${updatedTarget.deaths || 0} / ${kd})</div>`;
            
            // Show detailed stats for all players (teammates, enemies, and self)
            // Stats
            html += '<div style="margin-top: 4px;">';
            html += `Speed: ${Math.round(updatedTarget.speed)} `;
            html += `Dam: ${updatedTarget.damage}% `;
            html += `Sh: ${updatedTarget.shields}% `;
            html += `Fuel: ${updatedTarget.fuel}`;
            html += '</div>';
            
            // Status flags
            let status = [];
            if (updatedTarget.shields_up) status.push('Shields');
            if (updatedTarget.cloaked) status.push('Cloak');
            if (updatedTarget.wtemp > 50) status.push('W-Temp');
            if (updatedTarget.etemp > 50) status.push('E-Temp');
            if (updatedTarget.armies > 0) status.push(`${updatedTarget.armies} armies`);
            
            if (status.length > 0) {
                html += `<div style="margin-top: 4px;">${status.join(', ')}</div>`;
            }
        }
        
        // Update the element content
        this.element.innerHTML = html;
    }
    
    // Destroy the window
    destroy() {
        if (this.element) {
            this.element.remove();
            this.element = null;
        }
        if (this.timeout) {
            clearTimeout(this.timeout);
            this.timeout = null;
        }
        this.visible = false;
        this.currentTarget = null;
        this.targetType = null;
        this.playerIndex = -1;
    }
    
    // Check if window is visible
    isVisible() {
        return this.visible;
    }
}

// Create global instance
window.infoWindow = new InfoWindow();