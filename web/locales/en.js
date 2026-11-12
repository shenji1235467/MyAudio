/* English locale */
(function () {
  'use strict';
  window.I18N = window.I18N || {};
  window.I18N._locales = window.I18N._locales || {};
  window.I18N._locales['en'] = {
    // Common
    'app.title': 'MyAudio · Personal Audio Library',
    'common.loading': 'Loading…',
    'common.cancel': 'Cancel',
    'common.confirm': 'OK',
    'common.close': 'Close',
    'common.delete': 'Delete',
    'common.rootDir': 'Root',
    'common.cover': 'Cover',

    // Top navigation
    'nav.searchPlaceholder': 'Search title / artist / album…',
    'nav.gridView': 'Grid view',
    'nav.listView': 'List view',
    'nav.toggleTheme': 'Toggle light / dark theme',
    'nav.rescan': 'Rescan audio directory',
    'nav.playlists': 'Playlists',
    'nav.logout': 'Log out',
    'nav.switchLang': '切换为中文',
    'nav.more': 'More',
    'nav.folders': 'Folders',

    // Player
    'player.notPlaying': 'Not playing',
    'player.selectTrack': 'Select a track to start',
    'player.prev': 'Previous',
    'player.playPause': 'Play / pause',
    'player.next': 'Next',
    'player.mode': 'Play mode',
    'player.speed': 'Playback speed',
    'player.sleep': 'Sleep timer',
    'player.sleepTimer': 'Timer',
    'player.minutes': '{n} min',
    'player.playbackFailed': 'Playback failed: {msg}',
    'player.audioLoadFailed': 'Failed to load audio; the file may have been moved',
    'player.libraryEmpty': 'The audio library is empty',
    'player.speedLabel': 'Playback speed {speed}',
    'player.sleepEnded': 'Sleep timer ended, playback stopped',
    'player.sleepCancelled': 'Sleep timer cancelled',
    'player.sleepInMinutes': 'Playback will stop in {minutes} minutes',
    'player.resumeAt': 'Resume at {time}',

    // Play modes
    'mode.list': 'Repeat off',
    'mode.loop': 'Repeat all',
    'mode.random': 'Shuffle',

    // Library
    'library.title': 'Audio Library',
    'library.stats': '{count} tracks · Total {duration}',
    'library.scanning': ' · Scanning…',
    'library.orderLatest': 'Latest',
    'library.orderFilename': 'Filename',
    'library.orderTitle': 'Title',
    'library.orderArtist': 'Artist',
    'library.orderAlbum': 'Album',
    'library.emptyTitle': 'No audio yet',
    'library.emptyHint': 'Add MP3 / WAV / FLAC files to the audio directory, then click the scan button in the top-right corner',
    'library.loadMore': 'Load more',
    'library.unknownArtist': 'Unknown artist',
    'library.addToList': 'Add to list',
    'library.coverOf': '{title} cover',

    // Track deletion
    'track.deleteConfirm1': 'Remove this track record?\nClick "OK" to remove only the record. To also delete the file on disk, choose so in the next prompt.',
    'track.deleteConfirm2': 'Also delete the audio file on the server?\n(OK = delete file, Cancel = remove library record only)',
    'track.deleted': 'Deleted',

    // Theme
    'theme.toLight': 'Switch to light theme',
    'theme.toDark': 'Switch to dark theme',

    // Scan
    'scan.started': 'Scan started…',
    'scan.done': 'Scan complete',
    'scan.timeout': 'The scan is taking a while; please refresh manually later',
    'scan.inProgress': 'A scan is already in progress',

    // Logout
    'logout.confirm': 'Log out?',

    // Playlist drawer
    'pl.title': 'Playlists',
    'pl.new': 'New playlist',
    'pl.mine': 'My playlists',
    'pl.tracksSection': 'Playlist tracks',
    'pl.trackCount': '{count} tracks',
    'pl.deleteList': 'Delete playlist',
    'pl.empty': 'No playlists yet',
    'pl.deleteConfirm': 'Delete this playlist? (The tracks themselves will not be deleted.)',
    'pl.deleted': 'Playlist deleted',
    'pl.detailTitle': 'Playlist tracks · {count}',
    'pl.playAll': 'Play all',
    'pl.removeFromList': 'Remove from list',
    'pl.needCreateFirst': 'Please create a playlist first',
    'pl.addedTo': 'Added to "{name}"',
    'pl.createdAndAdded': 'Created and added to "{name}"',
    'pl.createdButAddFailed': 'Playlist created, but adding failed: {msg}',
    'pl.created': 'Playlist created',
    'pl.nameEmpty': 'Name cannot be empty',
    'pl.nameExists': 'A playlist with this name already exists',
    'pl.notFound': 'Record not found',

    // Modals
    'modal.newPlaylist': 'New playlist',
    'modal.newPlaylistAndAdd': 'New playlist & add track',
    'modal.nameLabel': 'Playlist name',
    'modal.namePlaceholder': 'e.g. Late-night drive',
    'modal.addToPlaylist': 'Add to playlist',
    'modal.newAndAdd': 'New playlist & add',

    // Login page
    'login.title': 'MyAudio · Sign in',
    'login.subtitle': 'Private online audio streaming',
    'login.passwordLabel': 'Access password',
    'login.passwordPlaceholder': 'Enter the access password',
    'login.submit': 'Enter library',
    'login.wrongPassword': 'Incorrect password, please try again',
    'login.foot': 'The session is kept via a cookie<br>5 consecutive failures lock access for 10 minutes',
    'login.enterPassword': 'Please enter the password',
    'login.signingIn': 'Signing in…',
    'login.failedHttp': 'Sign-in failed (HTTP {status})',
    'login.networkError': 'Network error; please check whether the service is available',
    'login.tooManyAttempts': 'Too many attempts, please try again in 10 minutes',
    'login.badRequest': 'Malformed request',
    'login.unauthorized': 'Not signed in or the session has expired',

    // Backend error codes
    'err.queryFailed': 'Query failed: {msg}',
    'err.trackNotFound': 'Track not found',
    'err.fileMissing': 'The audio file does not exist or has been moved',
    'err.invalidID': 'Invalid ID',
    'err.invalidTrackID': 'Invalid track ID',
    'err.emptyName': 'Name cannot be empty',
    'err.dirQueryFailed': 'Failed to query directories: {msg}',
    'err.invalidPosition': 'Invalid playback position',
  };
})();
