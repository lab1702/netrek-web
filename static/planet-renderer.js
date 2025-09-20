// Simplified planet renderer using circles with resource letters

class PlanetRenderer {
    constructor() {
        // Team colors for planets
        this.teamColors = {
            1: '#ffff00',  // Federation - Yellow
            2: '#ff0000',  // Romulan - Red
            4: '#00ff00',  // Klingon - Green
            8: '#00ffff',  // Orion - Cyan
            0: '#aaa',     // Nobody/Neutral - Light Gray
            '-1': '#aaa'   // Alternative for nobody
        };
    }
    
    // Draw resource letters inside planet circle
    drawResourceLetters(ctx, planet, x, y, radius) {
        if (!planet.flags) return;
        
        // Check for resources
        const hasAgri = (planet.flags & 64) !== 0;   // Agricultural
        const hasRepair = (planet.flags & 16) !== 0; // Repair  
        const hasFuel = (planet.flags & 32) !== 0;   // Fuel
        
        let letters = [];
        if (hasAgri) letters.push('A');
        if (hasRepair) letters.push('R');
        if (hasFuel) letters.push('F');
        
        if (letters.length === 0) return;
        
        // Use same color as planet outline for resource letters
        const isNeutral = planet.owner === 0 || planet.owner === -1;
        const textColor = isNeutral ? '#aaa' : (this.teamColors[planet.owner] || '#888');
        
        ctx.fillStyle = textColor;
        ctx.font = 'bold 6px monospace';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        
        // Position letters based on count
        if (letters.length === 1) {
            // Single letter in center
            ctx.fillText(letters[0], x, y);
        } else if (letters.length === 2) {
            // Two letters side by side
            ctx.fillText(letters[0], x - 3, y);
            ctx.fillText(letters[1], x + 3, y);
        } else if (letters.length === 3) {
            // Three letters in triangle formation
            ctx.fillText(letters[0], x, y - 3);
            ctx.fillText(letters[1], x - 3, y + 2);
            ctx.fillText(letters[2], x + 3, y + 2);
        }
    }
    
    // Draw a planet on the galactic map
    drawGalacticPlanet(ctx, planet, x, y, hasInfo = true) {
        const radius = 8;
        
        if (hasInfo) {
            // We have info - show actual planet with team colors
            const isNeutral = planet.owner === 0 || planet.owner === -1;
            const planetColor = isNeutral ? '#aaa' : (this.teamColors[planet.owner] || '#888');
            
            // Draw circle outline only
            ctx.strokeStyle = planetColor;
            ctx.lineWidth = 1;
            ctx.beginPath();
            ctx.arc(x, y, radius, 0, Math.PI * 2);
            ctx.stroke();
            
            // Draw resource letters inside circle
            this.drawResourceLetters(ctx, planet, x, y, radius);
            
            // Draw planet name below
            ctx.fillStyle = planetColor;
            ctx.font = '9px monospace';
            ctx.textAlign = 'center';
            ctx.textBaseline = 'top';
            ctx.fillText(planet.name.substring(0, 3).toUpperCase(), x, y + radius + 2);
        } else {
            // Unknown planet - always show as dark gray circle (unscouted)
            ctx.strokeStyle = '#444';
            ctx.lineWidth = 1;
            ctx.beginPath();
            ctx.arc(x, y, radius, 0, Math.PI * 2);
            ctx.stroke();
            
            // Draw question mark in center to indicate unknown
            ctx.fillStyle = '#444';
            ctx.font = 'bold 10px monospace';
            ctx.textAlign = 'center';
            ctx.textBaseline = 'middle';
            ctx.fillText('?', x, y);
            
            // Draw planet name below
            ctx.font = '9px monospace';
            ctx.textBaseline = 'top';
            ctx.fillText(planet.name.substring(0, 3).toUpperCase(), x, y + radius + 2);
        }
    }
    
    // Draw resource letters inside tactical planet circle (scaled version)
    drawResourceLettersTactical(ctx, planet, x, y, radius, scale) {
        if (!planet.flags) return;
        
        // Check for resources
        const hasAgri = (planet.flags & 64) !== 0;   // Agricultural
        const hasRepair = (planet.flags & 16) !== 0; // Repair  
        const hasFuel = (planet.flags & 32) !== 0;   // Fuel
        
        let letters = [];
        if (hasAgri) letters.push('A');
        if (hasRepair) letters.push('R');
        if (hasFuel) letters.push('F');
        
        if (letters.length === 0) return;
        
        // Use same color as planet outline for resource letters
        const isNeutral = planet.owner === 0 || planet.owner === -1;
        const textColor = isNeutral ? '#aaa' : (this.teamColors[planet.owner] || '#888');
        
        ctx.fillStyle = textColor;
        ctx.font = `bold ${10 * scale}px monospace`;
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        
        // Position letters based on count (scaled for tactical view)
        if (letters.length === 1) {
            // Single letter in center
            ctx.fillText(letters[0], x, y);
        } else if (letters.length === 2) {
            // Two letters side by side
            ctx.fillText(letters[0], x - 6 * scale, y);
            ctx.fillText(letters[1], x + 6 * scale, y);
        } else if (letters.length === 3) {
            // Three letters in triangle formation
            ctx.fillText(letters[0], x, y - 6 * scale);
            ctx.fillText(letters[1], x - 6 * scale, y + 4 * scale);
            ctx.fillText(letters[2], x + 6 * scale, y + 4 * scale);
        }
    }
    
    // Draw a planet on the tactical map
    drawTacticalPlanet(ctx, planet, x, y, hasInfo = true, scale = 1) {
        const radius = 20 * scale;
        
        if (hasInfo) {
            // We have info - show actual planet with team colors
            const isNeutral = planet.owner === 0 || planet.owner === -1;
            const planetColor = isNeutral ? '#aaa' : (this.teamColors[planet.owner] || '#888');
            
            // Draw circle outline only
            ctx.strokeStyle = planetColor;
            ctx.lineWidth = 2;
            ctx.beginPath();
            ctx.arc(x, y, radius, 0, Math.PI * 2);
            ctx.stroke();
            
            // Draw resource letters inside circle (scaled for tactical view)
            this.drawResourceLettersTactical(ctx, planet, x, y, radius, scale);
            
            // Draw planet name below
            ctx.fillStyle = planetColor;
            ctx.font = `${12 * scale}px monospace`;
            ctx.textAlign = 'center';
            ctx.textBaseline = 'top';
            ctx.fillText(planet.name, x, y + radius + 4 * scale);
        } else {
            // Unknown planet - always show as dark gray circle (unscouted)
            ctx.strokeStyle = '#444';
            ctx.lineWidth = 2;
            ctx.beginPath();
            ctx.arc(x, y, radius, 0, Math.PI * 2);
            ctx.stroke();
            
            // Draw question mark in center to indicate unknown
            ctx.fillStyle = '#444';
            ctx.font = `bold ${14 * scale}px monospace`;
            ctx.textAlign = 'center';
            ctx.textBaseline = 'middle';
            ctx.fillText('?', x, y);
            
            // Draw planet name (grayed out for unknown)
            ctx.font = `${12 * scale}px monospace`;
            ctx.textBaseline = 'top';
            ctx.fillText(planet.name, x, y + radius + 4 * scale);
        }
    }
}

// Create global instance
window.planetRenderer = new PlanetRenderer();
