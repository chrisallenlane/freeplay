var { describe, it } = require('node:test');
var assert = require('node:assert/strict');
var FP = require('./utils.js');

describe('stripExt', function () {
    it('removes a simple extension', function () {
        assert.equal(FP.stripExt('game.nes'), 'game');
    });

    it('removes only the last extension', function () {
        assert.equal(FP.stripExt('my.cool.game.zip'), 'my.cool.game');
    });

    it('returns the filename when there is no extension', function () {
        assert.equal(FP.stripExt('noext'), 'noext');
    });

    it('returns the filename when dot is at position 0 (hidden file)', function () {
        assert.equal(FP.stripExt('.hidden'), '.hidden');
    });

    it('handles empty string', function () {
        assert.equal(FP.stripExt(''), '');
    });
});

describe('favKey', function () {
    it('joins console and filename with slash', function () {
        assert.equal(FP.favKey({ console: 'SNES', filename: 'zelda.smc' }), 'SNES/zelda.smc');
    });

    it('preserves special characters', function () {
        assert.equal(
            FP.favKey({ console: 'Game Boy', filename: 'Pok\u00e9mon (USA).gb' }),
            'Game Boy/Pok\u00e9mon (USA).gb'
        );
    });
});

describe('filterGames', function () {
    var games = [
        { console: 'SNES', filename: 'Zelda.smc' },
        { console: 'SNES', filename: 'Mario.smc' },
        { console: 'NES', filename: 'Zelda.nes' },
        { console: 'NES', filename: 'Metroid.nes' },
    ];

    it('returns all games with no filters', function () {
        var result = FP.filterGames(games, {});
        assert.equal(result.length, 4);
    });

    it('filters by console', function () {
        var result = FP.filterGames(games, { console: 'NES' });
        assert.equal(result.length, 2);
        assert.ok(result.every(function (g) { return g.console === 'NES'; }));
    });

    it('filters by search query (case-insensitive)', function () {
        var result = FP.filterGames(games, { query: 'zelda' });
        assert.equal(result.length, 2);
        assert.ok(result.every(function (g) { return g.filename.toLowerCase().includes('zelda'); }));
    });

    it('filters by favorites', function () {
        var favs = new Set(['SNES/Mario.smc', 'NES/Metroid.nes']);
        var result = FP.filterGames(games, { favoritesOnly: true, favorites: favs });
        assert.equal(result.length, 2);
        assert.deepEqual(
            result.map(function (g) { return g.filename; }),
            ['Mario.smc', 'Metroid.nes']
        );
    });

    it('combines console and query filters', function () {
        var result = FP.filterGames(games, { console: 'SNES', query: 'zel' });
        assert.equal(result.length, 1);
        assert.equal(result[0].filename, 'Zelda.smc');
    });

    it('combines favorites and query filters', function () {
        var favs = new Set(['SNES/Zelda.smc', 'SNES/Mario.smc', 'NES/Zelda.nes']);
        var result = FP.filterGames(games, { favoritesOnly: true, favorites: favs, query: 'zelda' });
        assert.equal(result.length, 2);
    });

    it('returns empty array when nothing matches', function () {
        var result = FP.filterGames(games, { query: 'nonexistent' });
        assert.equal(result.length, 0);
    });

    it('treats empty query as no filter', function () {
        var result = FP.filterGames(games, { query: '' });
        assert.equal(result.length, 4);
    });
});

describe('findGame', function () {
    var games = [
        { console: 'SNES', filename: 'Zelda.smc' },
        { console: 'NES', filename: 'Zelda.nes' },
    ];

    it('finds a game by console and filename', function () {
        var game = FP.findGame(games, 'NES', 'Zelda.nes');
        assert.equal(game.console, 'NES');
        assert.equal(game.filename, 'Zelda.nes');
    });

    it('returns null when not found', function () {
        assert.equal(FP.findGame(games, 'NES', 'Mario.nes'), null);
    });

    it('requires both console and filename to match', function () {
        assert.equal(FP.findGame(games, 'SNES', 'Zelda.nes'), null);
    });
});

describe('coverUrl', function () {
    it('builds a cover URL with encoded components', function () {
        assert.equal(
            FP.coverUrl({ console: 'SNES', filename: 'Zelda.smc' }),
            '/covers/SNES/Zelda.png'
        );
    });

    it('encodes special characters', function () {
        assert.equal(
            FP.coverUrl({ console: 'Game Boy', filename: 'Pok\u00e9mon (USA).gb' }),
            '/covers/Game%20Boy/Pok%C3%A9mon%20(USA).png'
        );
    });
});

describe('playUrl', function () {
    it('builds a play URL with encoded components', function () {
        assert.equal(
            FP.playUrl({ console: 'SNES', filename: 'Zelda.smc' }),
            '/play?console=SNES&rom=Zelda.smc'
        );
    });

    it('encodes special characters', function () {
        assert.equal(
            FP.playUrl({ console: 'Game Boy', filename: 'Pok\u00e9mon (USA).gb' }),
            '/play?console=Game%20Boy&rom=Pok%C3%A9mon%20(USA).gb'
        );
    });
});

describe('romUrl', function () {
    it('builds a ROM URL', function () {
        assert.equal(FP.romUrl('SNES', 'Zelda.smc'), '/roms/SNES/Zelda.smc');
    });

    it('encodes special characters', function () {
        assert.equal(
            FP.romUrl('Game Boy', 'Pok\u00e9mon (USA).gb'),
            '/roms/Game%20Boy/Pok%C3%A9mon%20(USA).gb'
        );
    });
});

describe('saveBasePath', function () {
    it('builds a save base path', function () {
        assert.equal(FP.saveBasePath('SNES', 'Zelda'), '/api/saves/SNES/Zelda');
    });

    it('encodes special characters', function () {
        assert.equal(
            FP.saveBasePath('Game Boy', 'Pok\u00e9mon (USA)'),
            '/api/saves/Game%20Boy/Pok%C3%A9mon%20(USA)'
        );
    });
});

describe('biosUrl', function () {
    it('builds a BIOS URL', function () {
        assert.equal(FP.biosUrl('SNES'), '/bios/SNES/');
    });

    it('encodes special characters', function () {
        assert.equal(FP.biosUrl('Game Boy'), '/bios/Game%20Boy/');
    });
});
