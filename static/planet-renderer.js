// Simplified planet renderer using circles with resource letters

class PlanetRenderer {
    constructor() {
        // Team colors sourced from window.TEAM_COLORS (defined in netrek.js)
    }
    
    // Draw resource letters inside planet circle
    drawResourceLetters(ctx, planet, x, y, radius) {
        // Check for resources - always show all three positions
        const hasAgri = planet.flags ? (planet.flags & 64) !== 0 : false;   // Agricultural
        const hasRepair = planet.flags ? (planet.flags & 16) !== 0 : false; // Repair  
        const hasFuel = planet.flags ? (planet.flags & 32) !== 0 : false;   // Fuel
        
        // Build resource string with spaces for missing resources
        const resourceString = (hasAgri ? 'A' : ' ') + (hasRepair ? 'R' : ' ') + (hasFuel ? 'F' : ' ');
        
        // Only draw if there are any resources
        if (!hasAgri && !hasRepair && !hasFuel) return;
        
        // Use same color as planet outline for resource letters
        const isNeutral = planet.owner === 0 || planet.owner === -1;
        const textColor = isNeutral ? '#aaa' : (window.TEAM_COLORS[planet.owner] || '#888');
        
        ctx.fillStyle = textColor;
        ctx.font = 'bold 8px monospace';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        
        // Draw the full resource string centered
        ctx.fillText(resourceString, x, y);
    }
    
    // Draw a planet on the galactic map
    drawGalacticPlanet(ctx, planet, x, y, hasInfo = true) {
        const radius = 8;
        
        if (hasInfo) {
            // We have info - show actual planet with team colors
            const isNeutral = planet.owner === 0 || planet.owner === -1;
            const planetColor = isNeutral ? '#aaa' : (window.TEAM_COLORS[planet.owner] || '#888');
            
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
            ctx.fillText((planet.name || '???').substring(0, 3).toUpperCase(), x, y + radius + 2);
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
            ctx.fillText((planet.name || '???').substring(0, 3).toUpperCase(), x, y + radius + 2);
        }
    }
    
    // Draw resource letters inside tactical planet circle (scaled version)
    drawResourceLettersTactical(ctx, planet, x, y, radius, scale) {
        // Check for resources - always show all three positions
        const hasAgri = planet.flags ? (planet.flags & 64) !== 0 : false;   // Agricultural
        const hasRepair = planet.flags ? (planet.flags & 16) !== 0 : false; // Repair  
        const hasFuel = planet.flags ? (planet.flags & 32) !== 0 : false;   // Fuel
        
        // Build resource string with spaces for missing resources
        const resourceString = (hasAgri ? 'A' : ' ') + (hasRepair ? 'R' : ' ') + (hasFuel ? 'F' : ' ');
        
        // Only draw if there are any resources
        if (!hasAgri && !hasRepair && !hasFuel) return;
        
        // Use same color as planet outline for resource letters
        const isNeutral = planet.owner === 0 || planet.owner === -1;
        const textColor = isNeutral ? '#aaa' : (window.TEAM_COLORS[planet.owner] || '#888');
        
        ctx.fillStyle = textColor;
        ctx.font = `bold ${14 * scale}px monospace`;
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        
        // Draw the full resource string centered
        ctx.fillText(resourceString, x, y);
    }
    
    // Draw a planet on the tactical map
    drawTacticalPlanet(ctx, planet, x, y, hasInfo = true, scale = 1) {
        const radius = 20 * scale;
        
        if (hasInfo) {
            // We have info - show actual planet with team colors
            const isNeutral = planet.owner === 0 || planet.owner === -1;
            const planetColor = isNeutral ? '#aaa' : (window.TEAM_COLORS[planet.owner] || '#888');
            
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
            ctx.fillText(planet.name || '???', x, y + radius + 4 * scale);
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
            ctx.fillText(planet.name || '???', x, y + radius + 4 * scale);
        }
    }
}

// Create global instance
window.planetRenderer = new PlanetRenderer();
