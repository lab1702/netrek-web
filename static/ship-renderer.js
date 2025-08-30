// Ship Renderer - Based on original Netrek client rendering
// Handles ship sprite creation and caching for all teams, types, and directions

class ShipRenderer {
    constructor() {
        this.sprites = new Map(); // Cache for generated sprites
        this.initialized = false;
        this.teamColors = {
            0: '#888',  // Independent - gray
            1: '#ff0',  // Federation - yellow
            2: '#f00',  // Romulan - red
            4: '#0f0',  // Klingon - green
            8: '#0ff'   // Orion - cyan
        };
    }
    
    // Initialize and create all ship sprites
    async init() {
        if (this.initialized) return;
        
        // For now, just mark as initialized
        // We'll generate sprites on demand
        this.initialized = true;
    }
    
    // Get the rosette value (direction index) from angle in radians
    // Based on original client's rosette macro: ((((x) + 8) / 16) & 15)
    // where x is in 0-255 range, we need to convert from radians
    getRosette(radians) {
        // In original Netrek: 0 = north, 64 = east, 128 = south, 192 = west
        // In our system: 0 radians = east, pi/2 = south, pi = west, 3pi/2 = north
        // We need to add 90 degrees (add pi/2) to align properly
        let adjustedRadians = radians + Math.PI / 2;
        
        // Convert to degrees and normalize to 0-360
        let degrees = (adjustedRadians * 180 / Math.PI) % 360;
        if (degrees < 0) degrees += 360;
        
        // Convert to 0-255 range (0-255 represents full circle)
        let x = Math.floor(degrees * 256 / 360);
        
        // Apply rosette formula
        return ((x + 8) / 16) & 15;
    }
    
    // Generate sprite key for caching
    getSpriteKey(team, shipType, view) {
        return `${team}_${shipType}_${view}`;
    }
    
    // Get or create a ship sprite
    getShipSprite(team, shipType, direction) {
        const view = this.getRosette(direction);
        const key = this.getSpriteKey(team, shipType, view);
        
        // Check cache
        if (this.sprites.has(key)) {
            return this.sprites.get(key);
        }
        
        // Create new sprite
        const sprite = this.createShipSprite(team, shipType, view);
        if (sprite) {
            this.sprites.set(key, sprite);
        }
        return sprite;
    }
    
    // Create a ship sprite for a specific team, type, and view
    createShipSprite(team, shipType, view) {
        // Get team-specific bitmap data
        let shipData = null;
        
        if (window.teamBitmapMap && window.teamBitmapMap[team]) {
            // Use team-specific bitmaps
            const teamBitmaps = window.teamBitmapMap[team];
            shipData = teamBitmaps[shipType];
        } else if (window.allTeamShipBitmaps) {
            // Fallback to team name lookup
            const teamNames = {
                1: 'federation',
                2: 'romulan',
                4: 'klingon',
                8: 'orion'
            };
            const teamName = teamNames[team];
            if (teamName && window.allTeamShipBitmaps[teamName]) {
                shipData = window.allTeamShipBitmaps[teamName][shipType];
            }
        }
        
        // Final fallback to old system
        if (!shipData) {
            const bitmaps = window.allShipBitmaps || window.shipBitmaps;
            shipData = bitmaps.federation ? bitmaps.federation[shipType] : null;
        }
        
        if (!shipData || !shipData[view] || shipData[view].length === 0) {
            return null; // No bitmap data for this ship/view
        }
        
        const dimensions = window.shipDimensions[shipType];
        // Use team colors for the bitmaps
        const color = this.teamColors[team] || '#fff';
        
        // Convert bitmap to ImageData
        const imageData = window.convertShipBitmap(
            shipData[view],
            dimensions.width,
            dimensions.height,
            color
        );
        
        // Create canvas for this sprite
        const canvas = document.createElement('canvas');
        canvas.width = dimensions.width;
        canvas.height = dimensions.height;
        const ctx = canvas.getContext('2d');
        ctx.putImageData(imageData, 0, 0);
        
        return canvas;
    }
    
    // Draw a ship on the given context
    drawShip(ctx, player, x, y, scale = 1) {
        if (!this.initialized) return;
        
        const sprite = this.getShipSprite(player.team, player.ship, player.dir);
        if (!sprite) {
            // Fallback to simple triangle if no sprite
            this.drawFallbackShip(ctx, player, x, y, scale);
            return;
        }
        
        // Draw the sprite centered at x, y
        const width = sprite.width * scale;
        const height = sprite.height * scale;
        
        ctx.save();
        ctx.imageSmoothingEnabled = false; // Keep pixelated look
        ctx.drawImage(sprite, x - width/2, y - height/2, width, height);
        ctx.restore();
    }
    
    // Fallback ship drawing (simple triangle)
    drawFallbackShip(ctx, player, x, y, scale = 1) {
        const size = 10 * scale;
        const color = this.teamColors[player.team] || '#fff';
        
        ctx.save();
        ctx.translate(x, y);
        ctx.rotate(player.dir);
        
        ctx.fillStyle = color;
        ctx.strokeStyle = color;
        ctx.lineWidth = 1;
        
        ctx.beginPath();
        ctx.moveTo(0, -size);
        ctx.lineTo(-size/2, size);
        ctx.lineTo(size/2, size);
        ctx.closePath();
        ctx.fill();
        ctx.stroke();
        
        ctx.restore();
    }
}

// Create global instance
window.shipRenderer = new ShipRenderer();