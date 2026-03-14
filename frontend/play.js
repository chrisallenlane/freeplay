(function () {
    'use strict';

    var params = new URLSearchParams(window.location.search);
    var consoleName = params.get('console');
    var rom = params.get('rom');

    if (!consoleName || !rom) {
        showError('Missing console or rom parameter.');
        return;
    }

    var nameEl = document.getElementById('game-name');
    var dot = rom.lastIndexOf('.');
    nameEl.textContent = dot > 0 ? rom.substring(0, dot) : rom;
    document.title = 'Freeplay - ' + nameEl.textContent;

    fetch('/api/games')
        .then(function (res) { return res.json(); })
        .then(function (catalog) {
            var game = null;
            for (var i = 0; i < catalog.games.length; i++) {
                if (catalog.games[i].console === consoleName && catalog.games[i].filename === rom) {
                    game = catalog.games[i];
                    break;
                }
            }
            if (!game) {
                showError('Game not found. It may have been removed from the library.');
                return;
            }
            startEmulator(game);
        })
        .catch(function () {
            showError('Could not load game catalog.');
        });

    function showError(msg) {
        document.getElementById('game').style.display = 'none';
        var el = document.getElementById('error');
        el.style.display = '';
        el.textContent = msg;
    }

    function startEmulator(game) {
        var encConsole = encodeURIComponent(consoleName);
        var gameSlug = encodeURIComponent(nameEl.textContent);

        window.EJS_player = '#game';
        window.EJS_core = game.core;
        window.EJS_gameUrl = '/roms/' + encConsole + '/' + encodeURIComponent(rom);
        window.EJS_pathtodata = '/emulatorjs/data/';
        window.EJS_color = '#1a1a2e';
        window.EJS_gameName = nameEl.textContent;
        window.EJS_startOnLoaded = true;

        if (game.hasBios) {
            window.EJS_biosUrl = '/bios/' + encConsole + '/';
        }

        var saveBase = '/api/saves/' + encConsole + '/' + gameSlug;

        function postSave(type, data) {
            if (data) fetch(saveBase + '/' + type, { method: 'POST', body: new Blob([data]) });
        }

        window.EJS_onSaveState = function (data) { postSave('state', data.state); };
        window.EJS_onSaveSave = function (data) { postSave('sram', data.save); };

        // Register periodic SRAM save after game starts
        window.EJS_onGameStart = function () {
            if (window.EJS_emulator) {
                window.EJS_emulator.on('saveSaveFiles', function (data) { postSave('sram', data); });
            }
        };

        // Load save state if one exists, then start the emulator
        fetch(saveBase + '/state', { method: 'HEAD' })
            .then(function (res) {
                if (res.ok) {
                    window.EJS_loadStateURL = saveBase + '/state';
                }
            })
            .catch(function () {})
            .finally(function () {
                var script = document.createElement('script');
                script.src = '/emulatorjs/data/loader.js';
                document.body.appendChild(script);
            });
    }
})();
