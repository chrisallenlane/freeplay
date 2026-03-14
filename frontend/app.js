(function () {
    'use strict';

    let allGames = [];
    let consoles = [];
    let activeConsole = null;
    let activeFavorites = false;
    let favorites = new Set(JSON.parse(localStorage.getItem('freeplay-favorites') || '[]'));

    const grid = document.getElementById('game-grid');
    const filtersBar = document.getElementById('filters');
    const searchInput = document.getElementById('search');
    const rescanBtn = document.getElementById('rescan-btn');

    function saveFavorites() {
        localStorage.setItem('freeplay-favorites', JSON.stringify(Array.from(favorites)));
    }

    function favKey(game) {
        return game.console + '/' + game.filename;
    }

    function stripExt(filename) {
        const dot = filename.lastIndexOf('.');
        return dot > 0 ? filename.substring(0, dot) : filename;
    }

    function getFilteredGames() {
        const query = searchInput.value.toLowerCase();
        return allGames.filter(function (g) {
            if (activeFavorites && !favorites.has(favKey(g))) return false;
            if (activeConsole && g.console !== activeConsole) return false;
            if (query && !g.filename.toLowerCase().includes(query)) return false;
            return true;
        });
    }

    function renderAll() {
        renderFilters();
        renderGrid();
    }

    function renderFilters() {
        filtersBar.innerHTML = '';

        var favBtn = document.createElement('button');
        favBtn.className = 'filter-btn' + (activeFavorites ? ' active' : '');
        favBtn.textContent = '\u2605 Favorites';
        favBtn.addEventListener('click', function () {
            activeFavorites = !activeFavorites;
            if (activeFavorites) activeConsole = null;
            renderAll();
        });
        filtersBar.appendChild(favBtn);

        var allBtn = document.createElement('button');
        allBtn.className = 'filter-btn' + (!activeConsole && !activeFavorites ? ' active' : '');
        allBtn.textContent = 'All';
        allBtn.addEventListener('click', function () {
            activeConsole = null;
            activeFavorites = false;
            renderAll();
        });
        filtersBar.appendChild(allBtn);

        consoles.forEach(function (name) {
            var btn = document.createElement('button');
            btn.className = 'filter-btn' + (activeConsole === name ? ' active' : '');
            btn.textContent = name;
            btn.addEventListener('click', function () {
                activeConsole = name;
                activeFavorites = false;
                renderAll();
            });
            filtersBar.appendChild(btn);
        });
    }

    function renderGrid() {
        grid.innerHTML = '';
        var games = getFilteredGames();

        if (games.length === 0) {
            var msg = document.createElement('div');
            msg.className = 'message';
            if (allGames.length === 0) {
                msg.textContent = 'No games found. Add ROMs to your library and check your freeplay.toml configuration.';
            } else if (activeFavorites) {
                msg.textContent = 'No favorites yet. Click the star on a game to add it.';
            } else {
                msg.textContent = 'No games match your search.';
            }
            grid.appendChild(msg);
            return;
        }

        games.forEach(function (game) {
            var card = document.createElement('div');
            card.className = 'game-card';

            // Favorite button
            var isFav = favorites.has(favKey(game));
            var fav = document.createElement('button');
            fav.className = 'fav-btn' + (isFav ? ' favorited' : '');
            fav.textContent = isFav ? '\u2605' : '\u2606';
            fav.addEventListener('click', function (e) {
                e.stopPropagation();
                var key = favKey(game);
                if (favorites.has(key)) {
                    favorites.delete(key);
                    fav.textContent = '\u2606';
                    fav.classList.remove('favorited');
                } else {
                    favorites.add(key);
                    fav.textContent = '\u2605';
                    fav.classList.add('favorited');
                }
                saveFavorites();
                if (activeFavorites) renderGrid();
            });
            card.appendChild(fav);

            // Cover art or placeholder
            if (game.hasCover) {
                var img = document.createElement('img');
                img.className = 'cover';
                img.src = '/covers/' + encodeURIComponent(game.console) + '/' + encodeURIComponent(stripExt(game.filename)) + '.png';
                img.alt = game.filename;
                img.loading = 'lazy';
                card.appendChild(img);
            } else {
                var ph = document.createElement('div');
                ph.className = 'placeholder-cover';
                var phName = document.createElement('div');
                phName.className = 'placeholder-name';
                phName.textContent = stripExt(game.filename);
                var phConsole = document.createElement('div');
                phConsole.className = 'placeholder-console';
                phConsole.textContent = game.console;
                ph.appendChild(phName);
                ph.appendChild(phConsole);
                card.appendChild(ph);
            }

            // Card info
            var info = document.createElement('div');
            info.className = 'card-info';
            var title = document.createElement('div');
            title.className = 'card-title';
            title.textContent = stripExt(game.filename);
            var consoleName = document.createElement('div');
            consoleName.className = 'card-console';
            consoleName.textContent = game.console;
            info.appendChild(title);
            info.appendChild(consoleName);
            card.appendChild(info);

            // Click to play
            card.addEventListener('click', function () {
                window.location.href = '/play?console=' + encodeURIComponent(game.console) + '&rom=' + encodeURIComponent(game.filename);
            });

            grid.appendChild(card);
        });
    }

    function loadCatalog() {
        return fetch('/api/games')
            .then(function (res) {
                if (!res.ok) throw new Error('HTTP ' + res.status);
                return res.json();
            })
            .then(function (catalog) {
                allGames = catalog.games || [];
                consoles = catalog.consoles || [];
                renderAll();
            })
            .catch(function () {
                grid.innerHTML = '';
                var msg = document.createElement('div');
                msg.className = 'message';
                msg.textContent = 'Could not load game library. Check that Freeplay is running.';
                var retry = document.createElement('button');
                retry.textContent = 'Retry';
                retry.addEventListener('click', loadCatalog);
                msg.appendChild(retry);
                grid.appendChild(msg);
            });
    }

    searchInput.addEventListener('input', renderGrid);

    // Rescan button
    var statusPollTimer = null;

    function resetRescanBtn() {
        rescanBtn.disabled = false;
        rescanBtn.textContent = 'Rescan \u21BB';
        rescanBtn.classList.remove('fetching');
    }

    function pollCoverStatus() {
        fetch('/api/status')
            .then(function (res) { return res.json(); })
            .then(function (data) {
                if (data.fetchingCovers) {
                    rescanBtn.disabled = true;
                    rescanBtn.innerHTML = '<span class="spinner">\u21BB</span> Fetching covers\u2026';
                    rescanBtn.classList.add('fetching');
                    statusPollTimer = setTimeout(pollCoverStatus, 2000);
                } else {
                    resetRescanBtn();
                    loadCatalog();
                }
            })
            .catch(resetRescanBtn);
    }

    rescanBtn.addEventListener('click', function () {
        rescanBtn.disabled = true;
        rescanBtn.textContent = 'Scanning\u2026';
        fetch('/api/rescan', { method: 'POST' })
            .then(function (res) {
                if (res.status === 409) {
                    alert('Scan already in progress.');
                    return;
                }
                if (!res.ok) throw new Error('HTTP ' + res.status);
                return loadCatalog().then(pollCoverStatus);
            })
            .catch(function () {
                alert('Rescan failed. Check that Freeplay is running.');
            })
            .finally(function () {
                if (!statusPollTimer) {
                    resetRescanBtn();
                }
            });
    });

    loadCatalog();
})();
