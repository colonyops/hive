// Fills the hero download button and the install page's download panel from
// the release channel manifest on dl.hivedesktop.com. The bucket allows
// cross-origin reads from this site; without that rule the fetch fails and
// the fallbacks below apply.
//
// Everything here is an upgrade over markup that already works: the button
// starts as a link to the install section and the panel starts as a pointer at
// the install script, so a failed fetch, a blocked request, or no JavaScript
// leaves a page that still tells a visitor how to install.
(function () {
  // Every release writes the stable manifest, and the site deploys only
  // after a release has.
  var CHANNEL = "stable";

  // Artifacts are only ever linked from the release bucket. The manifest is
  // ours, but a download button is the one place a bad URL would matter.
  var ARTIFACT_ORIGIN = "https://dl.hivedesktop.com";
  var CHANNELS_BASE = ARTIFACT_ORIGIN + "/desktop/channels/";

  // Manifest platform keys, in the order the panel lists them. `match` picks
  // the one a download button offers, first match wins, so every entry has to
  // exclude the architectures below it rather than rely on its position.
  var PLATFORMS = [
    {
      key: "darwin-universal",
      name: "macOS",
      note: "Apple silicon and Intel",
      match: function (ua) {
        return /mac/i.test(ua);
      },
    },
    {
      key: "linux-amd64",
      name: "Linux",
      note: "x86-64",
      match: function (ua) {
        return isLinux(ua) && !/aarch64|arm64/i.test(ua);
      },
    },
    {
      key: "linux-arm64",
      name: "Linux",
      note: "arm64",
      match: function (ua) {
        return isLinux(ua) && /aarch64|arm64/i.test(ua);
      },
    },
  ];

  // Android reports Linux, and there is no Android build to offer it.
  function isLinux(ua) {
    return /linux/i.test(ua) && !/android/i.test(ua);
  }

  var pending = null;

  function manifest() {
    if (!pending) {
      pending = fetch(CHANNELS_BASE + encodeURIComponent(CHANNEL) + "/latest.json", {
        headers: { accept: "application/json" },
      })
        .then(function (response) {
          return response.ok ? response.json() : null;
        })
        .catch(function () {
          return null;
        });
    }
    return pending;
  }

  // The artifact a human downloads. On macOS that is the .dmg advertised in the
  // installer_* fields, not `url` — the zip is the updater's artifact and gives
  // a first-time visitor no install affordance. The three installer fields are
  // read as a set: a URL without its checksum would mean publishing a download
  // nobody can verify.
  function artifact(block) {
    if (!block) {
      return null;
    }
    var url = block.url;
    var sha256 = block.sha256;
    var size = block.size;
    if (block.installer_url && block.installer_sha256 && block.installer_size) {
      url = block.installer_url;
      sha256 = block.installer_sha256;
      size = block.installer_size;
    }
    if (!isArtifactURL(url)) {
      return null;
    }
    return { url: url, sha256: sha256, size: size, file: filename(url) };
  }

  function isArtifactURL(url) {
    if (typeof url !== "string" || url === "") {
      return false;
    }
    try {
      return new URL(url).origin === ARTIFACT_ORIGIN;
    } catch (error) {
      return false;
    }
  }

  function filename(url) {
    var path = new URL(url).pathname;
    return path.slice(path.lastIndexOf("/") + 1);
  }

  function megabytes(size) {
    return typeof size === "number" && size > 0 ? (size / 1e6).toFixed(1) + " MB" : "";
  }

  function release(data) {
    var version = typeof data.version === "string" ? data.version : "";
    var channel = typeof data.channel === "string" ? data.channel : CHANNEL;
    var label = version ? "v" + version : "";
    // A prerelease channel is named on the page. Someone downloading a dev
    // build should know that is what they are getting.
    if (label && channel !== "stable") {
      label += " · " + channel + " channel";
    }
    return label;
  }

  function available(data) {
    var platforms = (data && data.platforms) || {};
    return PLATFORMS.map(function (platform) {
      var art = artifact(platforms[platform.key]);
      return art ? { platform: platform, artifact: art } : null;
    }).filter(Boolean);
  }

  function element(tag, className, text) {
    var node = document.createElement(tag);
    if (className) {
      node.className = className;
    }
    if (text) {
      node.textContent = text;
    }
    return node;
  }

  function fillButtons(data) {
    var buttons = document.querySelectorAll("[data-hive-download]");
    if (!buttons.length) {
      return;
    }

    var ua = navigator.userAgent || "";
    var mine = available(data).filter(function (build) {
      return build.platform.match(ua);
    })[0];
    // No build for this visitor's platform: the buttons keep pointing at the
    // install section, which lists every platform that does have one.
    if (!mine) {
      return;
    }

    buttons.forEach(function (button) {
      button.href = mine.artifact.url;
      button.textContent = "Hive Desktop for " + mine.platform.name;
      // Cross-origin, and a release artifact is not a page to follow.
      button.setAttribute("rel", "noopener");
    });

    var meta = document.querySelector("[data-hive-download-meta]:not([data-hive-filled])");
    if (!meta) {
      return;
    }
    meta.setAttribute("data-hive-filled", "");
    var parts = [release(data), megabytes(mine.artifact.size)].filter(Boolean);
    if (parts.length) {
      meta.insertBefore(document.createTextNode(parts.join(" · ") + " · "), meta.firstChild);
    }
  }

  function fillPanel(data) {
    var panel = document.querySelector("[data-hive-downloads]");
    if (!panel) {
      return;
    }

    var builds = available(data);
    if (!builds.length) {
      return;
    }

    var fragment = document.createDocumentFragment();

    var label = release(data);
    if (label) {
      fragment.appendChild(element("p", "hive-downloads__release", label));
    }

    var list = element("div", "hive-downloads__list");
    builds.forEach(function (build) {
      var row = element("div", "hive-downloads__row");

      // The filename is deliberately not here. It is long, it pushes the row
      // onto a second line, and a visitor who wants it has it in the checksum
      // list below, paired with the hash it belongs to.
      var name = element("div", "hive-downloads__name");
      name.appendChild(element("strong", null, build.platform.name));
      name.appendChild(element("span", "hive-downloads__note", build.platform.note));
      row.appendChild(name);

      row.appendChild(element("span", "hive-downloads__size", megabytes(build.artifact.size)));

      var link = element("a", "md-button md-button--primary", "Download");
      link.href = build.artifact.url;
      link.setAttribute("rel", "noopener");
      row.appendChild(link);

      list.appendChild(row);
    });
    fragment.appendChild(list);

    var checksums = builds.filter(function (build) {
      return typeof build.artifact.sha256 === "string" && build.artifact.sha256 !== "";
    });
    if (checksums.length) {
      var details = element("details", "hive-downloads__checksums");
      details.appendChild(element("summary", null, "SHA-256 checksums"));
      var pre = element("pre");
      pre.appendChild(
        element(
          "code",
          null,
          checksums
            .map(function (build) {
              return build.artifact.sha256 + "  " + build.artifact.file;
            })
            .join("\n"),
        ),
      );
      details.appendChild(pre);
      fragment.appendChild(details);
    }

    panel.replaceChildren(fragment);
  }

  function fill() {
    if (!document.querySelector("[data-hive-download], [data-hive-downloads]")) {
      return;
    }
    manifest().then(function (data) {
      if (!data) {
        return;
      }
      fillButtons(data);
      fillPanel(data);
    });
  }

  // With instant navigation the page body is swapped in without a reload, so
  // the theme's document$ observable is the only reliable "page ready" hook.
  if (window.document$ && typeof window.document$.subscribe === "function") {
    window.document$.subscribe(fill);
  } else {
    fill();
  }
})();
