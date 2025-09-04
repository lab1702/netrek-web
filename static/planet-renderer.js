// Traditional Netrek planet renderer using original bitmaps

class PlanetRenderer {
    constructor() {
        this.sprites = {};
        // Team bit values from server
        this.teamColors = {
            1: '#ffff00',  // Federation (1 << 0) - Yellow
            2: '#ff0000',  // Romulan (1 << 1) - Red
            4: '#00ff00',  // Klingon (1 << 2) - Green
            8: '#00ffff',  // Orion (1 << 3) - Cyan
            0: '#888888',  // Nobody/Neutral - Gray
            '-1': '#888888' // Alternative for nobody
        };
        
        this.teamMap = {
            1: 0,  // Fed -> index 0
            2: 1,  // Rom -> index 1
            4: 2,  // Kli -> index 2
            8: 3,  // Ori -> index 3
            0: 4,  // Nobody -> index 4
            '-1': 4
        };
        
        this.teamNames = ['fed', 'rom', 'kli', 'ori', 'ind'];
        this.initialized = false;
    }
    
    // Initialize planet sprites
    async init() {
        if (this.initialized) return;
        
        // Check if bitmap data is already loaded
        if (window.planetBitmaps) {
            this.createAllSprites();
            this.initialized = true;
            return;
        }
        
        // Load bitmap data with timeout
        const script = document.createElement('script');
        script.src = 'convert_bitmaps.js';
        document.head.appendChild(script);
        
        await new Promise((resolve, reject) => {
            const timeout = setTimeout(() => {
                reject(new Error('Timeout loading bitmap data'));
            }, 3000);
            
            script.onload = () => {
                clearTimeout(timeout);
                resolve();
            };
            
            script.onerror = () => {
                clearTimeout(timeout);
                reject(new Error('Failed to load bitmap data'));
            };
        });
        
        // Create sprites for each team and resource combination
        this.createAllSprites();
        this.initialized = true;
    }
    
    createAllSprites() {
        // Create planet sprites
        
        // Map team index to actual team values
        const teamValues = [1, 2, 4, 8, 0]; // Fed, Rom, Kli, Ori, Nobody
        
        // Team-owned planets
        this.teamNames.forEach((team, index) => {
            const teamValue = teamValues[index];
            const color = this.teamColors[teamValue];
            const bitmapName = team + 'mplanet';
            
            if (window.planetBitmaps && window.planetBitmaps[bitmapName]) {
                this.sprites[bitmapName] = this.createSpriteCanvas(window.planetBitmaps[bitmapName], color);
            }
        });
        
        // Resource planets - create for each team color
        const resourceTypes = ['001', '010', '011', '100', '101', '110', '111'];
        
        this.teamNames.forEach((team, teamIndex) => {
            const teamValue = teamValues[teamIndex];
            const color = this.teamColors[teamValue];
            
            resourceTypes.forEach(resourceCode => {
                const spriteName = `${team}_mplanet${resourceCode}`;
                const bitmapName = `mplanet${resourceCode}`;
                
                if (window.planetBitmaps && window.planetBitmaps[bitmapName]) {
                    this.sprites[spriteName] = this.createSpriteCanvas(
                        window.planetBitmaps[bitmapName], 
                        color
                    );
                    // Resource sprite created
                }
            });
        });
        
        // Neutral resource planets
        resourceTypes.forEach(resourceCode => {
            const bitmapName = `mplanet${resourceCode}`;
            if (window.planetBitmaps && window.planetBitmaps[bitmapName]) {
                this.sprites[`ind_${bitmapName}`] = this.createSpriteCanvas(
                    window.planetBitmaps[bitmapName],
                    this.teamColors[0]  // Use 0 for TeamNone, not 4 (which is Klingon)
                );
            }
        });
        
        // Base neutral planet
        if (window.planetBitmaps && window.planetBitmaps.mplanet) {
            this.sprites['mplanet'] = this.createSpriteCanvas(window.planetBitmaps.mplanet, this.teamColors[0]);
            // Base mplanet sprite created
        }
    }
    
    createSpriteCanvas(bitmap, color) {
        const canvas = document.createElement('canvas');
        canvas.width = bitmap.width;
        canvas.height = bitmap.height;
        const ctx = canvas.getContext('2d');
        const imageData = this.convertBitmapToImageData(bitmap, color);
        ctx.putImageData(imageData, 0, 0);
        
        // Debug: Check if canvas has content
        const testData = ctx.getImageData(0, 0, canvas.width, canvas.height);
        let hasContent = false;
        for (let i = 3; i < testData.data.length; i += 4) {
            if (testData.data[i] > 0) {
                hasContent = true;
                break;
            }
        }
        if (!hasContent) {
            // Canvas has no visible content
        }
        
        return canvas;
    }
    
    convertBitmapToImageData(bitmap, color) {
        const { width, height, data } = bitmap;
        const canvas = document.createElement('canvas');
        canvas.width = width;
        canvas.height = height;
        const ctx = canvas.getContext('2d');
        const imageData = ctx.createImageData(width, height);
        
        // Parse color
        let r, g, b;
        if (color.startsWith('#')) {
            const hex = color.slice(1);
            r = parseInt(hex.substr(0, 2), 16);
            g = parseInt(hex.substr(2, 2), 16);
            b = parseInt(hex.substr(4, 2), 16);
        } else {
            r = g = b = 255;
        }
        
        // Convert bitmap data (2 bytes per row for 16-pixel width)
        for (let y = 0; y < height; y++) {
            const byteIndex = y * 2;
            const byte1 = data[byteIndex];
            const byte2 = data[byteIndex + 1];
            
            for (let x = 0; x < width; x++) {
                let bit;
                if (x < 8) {
                    bit = (byte1 >> x) & 1;
                } else {
                    bit = (byte2 >> (x - 8)) & 1;
                }
                
                const pixelIndex = (y * width + x) * 4;
                if (bit) {
                    imageData.data[pixelIndex] = r;
                    imageData.data[pixelIndex + 1] = g;
                    imageData.data[pixelIndex + 2] = b;
                    imageData.data[pixelIndex + 3] = 255;
                } else {
                    imageData.data[pixelIndex + 3] = 0; // Transparent
                }
            }
        }
        
        return imageData;
    }
    
    // Get the appropriate sprite for a planet
    getPlanetSprite(planet, showResources = true) {
        if (!this.initialized) {
            // Planet renderer not initialized
            return null;
        }
        
        let spriteName;
        const teamIndex = this.teamMap[planet.owner] !== undefined ? this.teamMap[planet.owner] : 4;
        const teamName = this.teamNames[teamIndex] || 'ind';
        
        
        if (showResources && planet.flags !== undefined) {
            // Decode planet flags (from server game/types.go)
            // PlanetRepair = 1 << 4 (16)
            // PlanetFuel   = 1 << 5 (32)
            // PlanetAgri   = 1 << 6 (64)
            
            const hasRepair = (planet.flags & 16) !== 0;
            const hasFuel = (planet.flags & 32) !== 0;
            const hasAgri = (planet.flags & 64) !== 0;
            
            // Build resource code (3-bit: AGRI|REPAIR|FUEL)
            let resourceCode = '';
            resourceCode += hasAgri ? '1' : '0';     // Agricultural
            resourceCode += hasRepair ? '1' : '0';   // Repair
            resourceCode += hasFuel ? '1' : '0';     // Fuel
            
            if (resourceCode !== '000') {
                spriteName = `${teamName}_mplanet${resourceCode}`;
            } else {
                // No resources - use base planet bitmap
                spriteName = `${teamName}mplanet`;
            }
        } else {
            // No flags defined - use base planet bitmap
            spriteName = `${teamName}mplanet`;
        }
        
        
        const sprite = this.sprites[spriteName] || this.sprites['mplanet'];
        
        return sprite;
    }
    
    // Helper to draw empty planet circle
    drawEmptyPlanetCircle(ctx, planet, x, y) {
        // Draw circle outline - slightly smaller than 16x16 to match bitmap visual size
        // Bitmaps have a circle that's about 14 pixels diameter
        ctx.strokeStyle = this.teamColors[planet.owner] || '#888';
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.arc(x, y, 6.5, 0, Math.PI * 2);
        ctx.stroke();
    }
    
    // Draw a planet on the galactic map
    drawGalacticPlanet(ctx, planet, x, y) {
        const sprite = this.getPlanetSprite(planet, true);
        if (sprite) {
            // Draw bitmap centered at position
            try {
                ctx.drawImage(sprite, x - 8, y - 8);
            } catch (e) {
                // Failed to draw sprite
                // Fallback on error
                this.drawEmptyPlanetCircle(ctx, planet, x, y);
            }
        } else {
            // Draw empty circle for planets without resources
            this.drawEmptyPlanetCircle(ctx, planet, x, y);
        }
        
        // Draw planet name below
        ctx.fillStyle = this.teamColors[planet.owner] || '#888';
        ctx.font = '9px monospace';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'top';
        ctx.fillText(planet.name.substring(0, 3).toUpperCase(), x, y + 10);
        
        // Draw independent indicator (crossing lines) for neutral planets
        if (planet.owner === 0) {
            ctx.strokeStyle = '#fff';
            ctx.lineWidth = 1;
            ctx.globalAlpha = 0.5;
            ctx.beginPath();
            ctx.moveTo(x - 6, y - 6);
            ctx.lineTo(x + 6, y + 6);
            ctx.moveTo(x - 6, y + 6);
            ctx.lineTo(x + 6, y - 6);
            ctx.stroke();
            ctx.globalAlpha = 1;
        }
    }
    
    // Draw a planet on the tactical map (would use 30x30 bitmaps)
    drawTacticalPlanet(ctx, planet, x, y, scale = 1) {
        // For now, use galactic sprites scaled up
        // In a full implementation, we'd have separate 30x30 tactical sprites
        const sprite = this.getPlanetSprite(planet, true);
        
        if (sprite) {
            ctx.save();
            ctx.translate(x, y);
            // Scale up more for tactical - original was 16x16, we want about 40x40
            ctx.scale(scale * 2.5, scale * 2.5); 
            ctx.drawImage(sprite, -8, -8); // Center the 16x16 sprite
            ctx.restore();
        } else {
            // Draw empty circle for planets without resources
            const radius = 20 * scale;
            ctx.strokeStyle = this.teamColors[planet.owner] || '#888';
            ctx.lineWidth = 2;
            ctx.beginPath();
            ctx.arc(x, y, radius, 0, Math.PI * 2);
            ctx.stroke();
        }
        
        // Draw planet name
        ctx.fillStyle = this.teamColors[planet.owner] || '#888';
        ctx.font = `${12 * scale}px monospace`;
        ctx.textAlign = 'center';
        ctx.textBaseline = 'top';
        ctx.fillText(planet.name, x, y + 20 * scale);
    }
}

// Create global instance
window.planetRenderer = new PlanetRenderer();

// Test function to debug sprite rendering
window.testPlanetSprites = function() {
    const canvas = document.createElement('canvas');
    canvas.width = 400;
    canvas.height = 200;
    canvas.style.position = 'fixed';
    canvas.style.top = '10px';
    canvas.style.right = '10px';
    canvas.style.border = '2px solid lime';
    canvas.style.background = 'black';
    canvas.style.zIndex = '10000';
    document.body.appendChild(canvas);
    
    const ctx = canvas.getContext('2d');
    ctx.fillStyle = 'white';
    ctx.font = '10px monospace';
    ctx.fillText('Planet Sprite Test:', 10, 15);
    
    let x = 10;
    let y = 30;
    let count = 0;
    
    // Try to draw each sprite
    for (const [name, sprite] of Object.entries(window.planetRenderer.sprites)) {
        if (sprite) {
            // Draw white background to see sprite bounds
            ctx.fillStyle = '#222';
            ctx.fillRect(x, y, 16, 16);
            
            ctx.drawImage(sprite, x, y);
            
            // Check if sprite has any visible pixels
            const testData = ctx.getImageData(x, y, 16, 16);
            let hasPixels = false;
            for (let i = 3; i < testData.data.length; i += 4) {
                if (testData.data[i] > 0) {
                    hasPixels = true;
                    break;
                }
            }
            
            ctx.fillStyle = hasPixels ? 'lime' : 'red';
            ctx.font = '8px monospace';
            ctx.fillText(name.substring(0, 8), x - 5, y + 25);
            
            x += 25;
            count++;
            if (count % 15 === 0) {
                x = 10;
                y += 35;
            }
        }
    }
    
    ctx.fillStyle = 'yellow';
    ctx.font = '10px monospace';
    ctx.fillText(`Total sprites: ${Object.keys(window.planetRenderer.sprites).length}`, 10, 190);
    
    if (Object.keys(window.planetRenderer.sprites).length === 0) {
        ctx.fillStyle = 'red';
        ctx.fillText('No sprites loaded!', 10, 40);
    }
    
    // Test a simple manual bitmap
    ctx.fillStyle = 'cyan';
    ctx.fillText('Manual test:', 250, 15);
    if (window.planetBitmaps && window.planetBitmaps.mplanet) {
        const testCanvas = document.createElement('canvas');
        testCanvas.width = 16;
        testCanvas.height = 16;
        const testCtx = testCanvas.getContext('2d');
        
        // Manually draw the bitmap
        const bitmap = window.planetBitmaps.mplanet;
        for (let y = 0; y < 16; y++) {
            const byte1 = bitmap.data[y * 2];
            const byte2 = bitmap.data[y * 2 + 1];
            for (let x = 0; x < 16; x++) {
                let bit;
                if (x < 8) {
                    bit = (byte1 >> x) & 1;
                } else {
                    bit = (byte2 >> (x - 8)) & 1;
                }
                if (bit) {
                    testCtx.fillStyle = 'white';
                    testCtx.fillRect(x, y, 1, 1);
                }
            }
        }
        
        ctx.drawImage(testCanvas, 250, 30);
        ctx.fillText('Direct draw', 250, 50);
    }
};