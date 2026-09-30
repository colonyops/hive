// Swaps a landing-page demo player for its placeholder when the video file
// is missing, so the page never shows an empty player while a demo is still
// being recorded.
(function () {
  function markMissing(figure) {
    figure.classList.add("hive-demo--missing");
  }

  function watch() {
    document.querySelectorAll(".hive-demo").forEach(function (figure) {
      var video = figure.querySelector("video");
      if (!video) {
        return;
      }

      var sources = video.querySelectorAll("source");
      var last = sources.length ? sources[sources.length - 1] : video;
      last.addEventListener("error", function () {
        markMissing(figure);
      });

      if (video.error || video.networkState === HTMLMediaElement.NETWORK_NO_SOURCE) {
        markMissing(figure);
      }
    });
  }

  // With instant navigation the page body is swapped in without a reload, so
  // the theme's document$ observable is the only reliable "page ready" hook.
  if (window.document$ && typeof window.document$.subscribe === "function") {
    window.document$.subscribe(watch);
  } else {
    watch();
  }
})();
