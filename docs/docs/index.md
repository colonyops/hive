---
icon: fontawesome/brands/hive
hide:
  - toc
---

<section class="hive-hero">
  <div class="hive-eyebrow">Local-first · Open source</div>
  <h1>The layer above your coding agents.</h1>
  <p class="hive-lede">The hive CLI runs Claude Code, Codex, Pi, or any terminal agent in its own git checkout and tmux session, and keeps status, context, tasks, and messages in one tree. Hive Desktop adds an inbox that collects pull requests, issues, and alerts into feeds you design, hands any item to an agent, and hosts chat workspaces for work outside a repository.</p>
  <div class="hive-hero__actions">
    <a class="md-button md-button--primary" data-hive-download href="desktop/getting-started/#install">Download Hive Desktop</a>
    <a class="md-button" href="cli/getting-started/">Install the CLI</a>
  </div>
  <p class="hive-hero__meta" data-hive-download-meta><a href="desktop/getting-started/#install">All downloads</a></p>
  <div class="hive-install">
    <pre><code>curl -fsSL https://hivedesktop.com/install.sh | bash   # Hive Desktop
brew install colonyops/tap/hive                        # hive CLI</code></pre>
  </div>
  <p class="hive-hero__note">MIT licensed · macOS and Linux</p>
</section>

<section class="hive-strip">
  <a href="#cli"><span class="hive-strip__icon" aria-hidden="true"><svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" viewBox="0 0 24 24"><path d="M12 19h8M4 17l6-6-6-6"/></svg></span><strong>CLI</strong><span class="hive-strip__desc">Give each agent its own checkout and tmux session, and coordinate above them from one tree.</span></a>
  <a href="#feeds"><span class="hive-strip__icon" aria-hidden="true"><svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" viewBox="0 0 24 24"><path d="M22 12h-6l-2 3h-4l-2-3H2"/><path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/></svg></span><strong>Feeds</strong><span class="hive-strip__desc">Gather scattered work into feeds you design, evaluate it once, and decide what happens next.</span></a>
  <a href="#code"><span class="hive-strip__icon" aria-hidden="true"><svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" viewBox="0 0 24 24"><path d="m7 11 2-2-2-2"/><path d="M11 13h4"/><rect width="18" height="18" x="3" y="3" rx="2" ry="2"/></svg></span><strong>Code</strong><span class="hive-strip__desc">Run several streams of agent work at once from the desktop, on the same session engine.</span></a>
  <a href="#chats"><span class="hive-strip__icon" aria-hidden="true"><svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" viewBox="0 0 24 24"><path d="M12 8V4H8"/><rect width="16" height="12" x="4" y="8" rx="2"/><path d="M2 14h2"/><path d="M20 14h2"/><path d="M15 13v2"/><path d="M9 13v2"/></svg></span><strong>Chats</strong><span class="hive-strip__desc">Give an agent a workspace with skills and MCP servers for work that is not code.</span></a>
</section>

<section class="hive-showcase" id="cli">
  <div class="hive-showcase__copy">
    <div class="hive-eyebrow"><svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" viewBox="0 0 24 24"><path d="M12 19h8M4 17l6-6-6-6"/></svg>hive CLI</div>
    <h2>Give every task its own room.</h2>
    <p>The hive CLI is a tmux-native command center. It clones a repository for each session, or checks out a worktree, starts your agent in a tmux window, and shows every session's status in one tree. Sessions keep running when you close the TUI.</p>
    <ul>
      <li><strong>Bring your harness.</strong> Claude Code, Codex, Pi, or any terminal agent. Hive leaves their prompts, permissions, and tool calls alone.</li>
      <li><strong>Shared context.</strong> A repo-scoped <code>.hive</code> directory holds plans, research, and handoffs that every session can read.</li>
      <li><strong>Tasks and messages.</strong> Built-in epics, tasks, and blockers, plus pub/sub inboxes so agents can hand work to each other.</li>
      <li><strong>Your commands in the palette.</strong> Bind keys and commands to scripts, review flows, dev servers, and test runners.</li>
    </ul>
    <a class="hive-showcase__link" href="cli/getting-started/">Get started with the CLI</a>
  </div>
  <div class="hive-terminal-demo" role="img" aria-label="Animated terminal demo of creating Hive sessions">
    <div class="hive-terminal-demo__bar" aria-hidden="true">
      <span></span><span></span><span></span>
      <strong>hive</strong>
    </div>
    <div class="hive-terminal-demo__screen" aria-hidden="true">
      <div class="term-shell">
        <div class="term-line term-command term-command--one"><span class="term-prompt">$</span> <span class="term-typed">hive new auth-refactor --remote https://github.com/acme/app.git --background</span></div>
        <div class="term-line term-output term-output--one">hook [1/2] mise install</div>
        <div class="term-line term-output term-output--two">mise all tools are installed</div>
        <div class="term-line term-output term-output--three">hook [2/2] hive ctx init</div>
        <div class="term-line term-output term-output--four">Created symlink: .hive -&gt; ~/.local/share/hive/context/acme/app</div>
        <div class="term-line term-output term-output--five">Session created</div>
        <div class="term-line term-output term-output--six">  ~/.local/share/hive/repos/app-a1b2c3</div>
        <div class="term-line term-command term-command--two"><span class="term-prompt">$</span> <span class="term-typed">hive</span></div>
      </div>
      <div class="term-tui">
        <div class="term-line term-output term-ui">colonyops/hive</div>
        <div class="term-line term-output term-ui term-line--status">  <b>[●]</b> docs-landing-page</div>
        <div class="term-line term-output term-ui term-line--status">      ├─ <b>[●]</b> claude</div>
        <div class="term-line term-output term-ui term-line--status">      └─ <b>[&gt;]</b> shell</div>
        <div class="term-line term-output term-ui">  [!] ci-fix</div>
        <div class="term-line term-output term-ui">      └─ [!] codex</div>
        <div class="term-line term-output term-ui">acme/app</div>
        <div class="term-line term-output term-ui term-line--status">  <b>[●]</b> auth-refactor</div>
        <div class="term-line term-output term-ui term-line--status">      ├─ <b>[●]</b> claude</div>
        <div class="term-line term-output term-ui term-line--status">      └─ <b>[&gt;]</b> shell</div>
        <div class="term-line term-output term-ui">  [?] payment-tests</div>
      </div>
    </div>
  </div>
</section>

<section class="hive-showcase hive-showcase--flip" id="feeds">
  <div class="hive-showcase__copy">
    <div class="hive-eyebrow"><svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" viewBox="0 0 24 24"><path d="M22 12h-6l-2 3h-4l-2-3H2"/><path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/></svg>Feeds</div>
    <h2>Bring scattered work into feeds you design.</h2>
    <p>Hive Desktop collects from GitHub, Gitea, Grafana, PostHog, local webhooks, and any command that prints JSON. Flows route every item into the feed where you will look at it, so you evaluate once and decide what to do.</p>
    <ul>
      <li><strong>Any data, any feed.</strong> Filters cover the common rules. JavaScript function nodes parse and reshape any payload, split it across outputs, and route it into whatever feed structure you want.</li>
      <li><strong>One ping per real change.</strong> Notify nodes deduplicate and cool down, so a firing alert or a moving pull request interrupts you once.</li>
      <li><strong>From item to agent.</strong> A launch-session action starts a repository coding session or sends the item to an agent workspace as its opening prompt.</li>
    </ul>
    <a class="hive-showcase__link" href="desktop/inbox/flows/">How flows work</a>
  </div>
  <figure class="hive-demo">
    <div class="hive-demo__bar" aria-hidden="true"><span></span><span></span><span></span><strong>Feeds</strong></div>
    <video class="hive-demo__video" controls playsinline preload="metadata" aria-label="Demo: working through feeds in Hive Desktop">
      <source src="assets/demos/feeds.mp4" type="video/mp4">
    </video>
    <div class="hive-demo__placeholder" aria-hidden="true">Demo video coming soon</div>
  </figure>
</section>

<section class="hive-showcase" id="code">
  <div class="hive-showcase__copy">
    <div class="hive-eyebrow"><svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" viewBox="0 0 24 24"><path d="m7 11 2-2-2-2"/><path d="M11 13h4"/><rect width="18" height="18" x="3" y="3" rx="2" ry="2"/></svg>Code</div>
    <h2>Run several streams of work at once.</h2>
    <p>Code is built on the <a href="cli/getting-started/sessions/">hive CLI session engine</a> and tmux. Each session gets its own checkout and windows, keeps running when Hive closes, and stays visible to an existing hive install and to <code>tmux attach</code>.</p>
    <ul>
      <li><strong>One shared session model.</strong> By default, sessions created in Hive Desktop or the hive CLI appear in both. Agent profiles, clone strategies, setup commands, and starting windows come from <a href="cli/configuration/">hive CLI configuration</a>.</li>
      <li><strong>A sidebar for every session.</strong> Attach to any window, switch with <kbd>⌘1</kbd> through <kbd>⌘9</kbd>, filter sessions, and search scrollback.</li>
      <li><strong>Your commands in the menus.</strong> Actions add commands to session and window menus. Quick terminals open tools such as lazygit in the active checkout.</li>
      <li><strong>Scratch terminals.</strong> A tmux session for shells that belong to no repository, with tabs you can rename and reorder.</li>
    </ul>
    <a class="hive-showcase__link" href="desktop/code/terminal-mode/">Terminal mode</a>
  </div>
  <figure class="hive-demo">
    <div class="hive-demo__bar" aria-hidden="true"><span></span><span></span><span></span><strong>Code</strong></div>
    <video class="hive-demo__video" controls playsinline preload="metadata" aria-label="Demo: running coding agent sessions in Hive Desktop">
      <source src="assets/demos/code.mp4" type="video/mp4">
    </video>
    <div class="hive-demo__placeholder" aria-hidden="true">Demo video coming soon</div>
  </figure>
</section>

<section class="hive-showcase hive-showcase--flip" id="chats">
  <div class="hive-showcase__copy">
    <div class="hive-eyebrow"><svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" viewBox="0 0 24 24"><path d="M12 8V4H8"/><rect width="16" height="12" x="4" y="8" rx="2"/><path d="M2 14h2"/><path d="M20 14h2"/><path d="M15 13v2"/><path d="M9 13v2"/></svg>Chats</div>
    <h2>Capture context for work that is not code.</h2>
    <p>A chat is a named workspace with an agent, skill packages, and MCP servers. Point one at Slack threads, call transcripts, and email to learn how people feel about your product and what to build next. Give another OpenSCAD skills and a render script that writes G-code, and prototype a 3D print from a prompt.</p>
    <ul>
      <li><strong>Built to your spec.</strong> Each workspace chooses Claude Code or Codex, an approval mode, skills, and MCP servers, and keeps its instructions in an <code>AGENTS.md</code> file.</li>
      <li><strong>The Hive workspace.</strong> Hive ships a workspace that knows its own configuration. Ask it for a feed, an action, a shortcut, or a setting, and review the file it writes.</li>
      <li><strong>An MCP server for the app.</strong> Any agent can connect to Hive's local MCP server to read feeds, refresh sources, and dry-run a flow against a payload.</li>
    </ul>
    <a class="hive-showcase__link" href="desktop/chats/agent-workspaces/">Agent workspaces</a>
  </div>
  <figure class="hive-demo">
    <div class="hive-demo__bar" aria-hidden="true"><span></span><span></span><span></span><strong>Chats</strong></div>
    <video class="hive-demo__video" controls playsinline preload="metadata" aria-label="Demo: agent workspaces in Hive Desktop">
      <source src="assets/demos/chats.mp4" type="video/mp4">
    </video>
    <div class="hive-demo__placeholder" aria-hidden="true">Demo video coming soon</div>
  </figure>
</section>

<div class="hive-cta">
  <div>
    <h2>Install Hive and give your agents a shared room to work in.</h2>
    <p>Start with the CLI in your terminal, or download the desktop app and build your first feed.</p>
  </div>
  <div class="hive-cta__actions">
    <a class="md-button md-button--primary" data-hive-download href="desktop/getting-started/#install">Download Hive Desktop</a>
    <a class="md-button" href="cli/getting-started/">Install the CLI</a>
  </div>
</div>

---

<small>LLM-friendly: [llms.txt](llms.txt) | [llms-full.txt](llms-full.txt)</small>
