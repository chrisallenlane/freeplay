(function (exports) {
    'use strict';

    exports.stripExt = function (filename) {
        var dot = filename.lastIndexOf('.');
        return dot > 0 ? filename.substring(0, dot) : filename;
    };

    exports.favKey = function (game) {
        return game.console + '/' + game.filename;
    };

    exports.filterGames = function (games, opts) {
        var query = (opts.query || '').toLowerCase();
        return games.filter(function (g) {
            if (opts.favoritesOnly && !opts.favorites.has(exports.favKey(g))) return false;
            if (opts.console && g.console !== opts.console) return false;
            if (query && !g.filename.toLowerCase().includes(query)) return false;
            return true;
        });
    };

    exports.findGame = function (games, consoleName, filename) {
        for (var i = 0; i < games.length; i++) {
            if (games[i].console === consoleName && games[i].filename === filename) {
                return games[i];
            }
        }
        return null;
    };

    exports.coverUrl = function (game) {
        return '/covers/' + encodeURIComponent(game.console) + '/' + encodeURIComponent(exports.stripExt(game.filename)) + '.png';
    };

    exports.playUrl = function (game) {
        return '/play?console=' + encodeURIComponent(game.console) + '&rom=' + encodeURIComponent(game.filename);
    };

    exports.romUrl = function (consoleName, rom) {
        return '/roms/' + encodeURIComponent(consoleName) + '/' + encodeURIComponent(rom);
    };

    exports.saveBasePath = function (consoleName, gameSlug) {
        return '/api/saves/' + encodeURIComponent(consoleName) + '/' + encodeURIComponent(gameSlug);
    };

    exports.biosUrl = function (consoleName) {
        return '/bios/' + encodeURIComponent(consoleName) + '/';
    };

})(typeof module !== 'undefined' ? module.exports : (window.Freeplay = {}));
