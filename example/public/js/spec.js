// Toggles the spec rail — the margin column naming the controller, policy or job
// behind each page — and remembers the choice in localStorage across visits.
(function () {
  var KEY = 'genesys.spec';
  function sync(off) {
    document.body.classList.toggle('spec-off', off);
    document.querySelectorAll('[data-spec-toggle]').forEach(function (el) {
      el.setAttribute('aria-pressed', off ? 'false' : 'true');
    });
  }

  function start() {
    var saved = null;
    try { saved = localStorage.getItem(KEY); } catch (e) {} // a private window can throw
    sync(saved === 'off');

    document.addEventListener('click', function (event) {
      var el = event.target.closest && event.target.closest('[data-spec-toggle]');
      if (!el) return;
      var off = !document.body.classList.contains('spec-off');
      sync(off);
      try { localStorage.setItem(KEY, off ? 'off' : 'on'); } catch (e) {}
    });
  }

  // Loaded with defer, so DOMContentLoaded may already have fired by now.
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start);
  else start();
})();
