---
icon: fontawesome/brands/hive
hide:
  - toc
---

<section class="hive-hero">
  <div class="hive-hero__copy">
    <div class="hive-eyebrow">Tmux-native metaharness</div>
    <h1>The layer above your coding agents.</h1>
    <p class="hive-lede">Hive does not replace <a href="https://docs.anthropic.com/en/docs/claude-code/overview" target="_blank" rel="noopener noreferrer">Claude Code</a>, <a href="https://openai.com/codex/" target="_blank" rel="noopener noreferrer">Codex</a>, <a href="https://pi.dev" target="_blank" rel="noopener noreferrer">Pi</a>, or your shell. It gives them a shared tmux command center with isolated git workspaces or worktrees, live status, repo context, task coordination, and inter-agent messaging.</p>
    <div class="hive-hero__actions">
      <a class="md-button md-button--primary" href="getting-started/">Get started</a>
      <a class="md-button" href="https://github.com/colonyops/hive" target="_blank" rel="noopener noreferrer">View on GitHub</a>
    </div>
    <div class="hive-install">
      <pre><code>brew install tmux
brew install colonyops/tap/hive</code></pre>
    </div>
  </div>

  <div class="hive-terminal-demo" aria-label="Animated terminal demo of creating Hive sessions">
    <div class="hive-terminal-demo__bar">
      <span></span><span></span><span></span>
      <strong>hive</strong>
    </div>
    <div class="hive-terminal-demo__screen">
      <div class="term-shell">
        <div class="term-line term-command term-command--one"><span class="term-prompt">$</span> <span class="term-typed">hive new auth-refactor --remote github.com/acme/app --background</span></div>
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

<section class="hive-strip">
  <div><strong>Bring your harness</strong><span>Use Claude Code, Codex, Pi, or any terminal agent.</span></div>
  <div><strong>Give each task a room</strong><span>Run work in isolated clones or worktrees with their own tmux sessions.</span></div>
  <div><strong>Coordinate above it</strong><span>Share status, context, tasks, notes, and messages through Hive.</span></div>
</section>

<section class="hive-metaharness-section">
  <div class="hive-metaharness-section__inner">
    <h2>The Metaharness Layer</h2>
    <p>Coding tools already have strong harnesses: prompts, permissions, tool calls, auth, and terminal behavior. Hive leaves those loops alone. It sits above tmux to manage sessions, show what is happening, and help agents communicate.</p>

    <div class="hive-layer-diagram" aria-label="Hive layered above tmux sessions and agents">
      <div class="hive-layer hive-layer--hive">
        <strong>Hive</strong>
        <span>command center · status · context · tasks · messaging</span>
      </div>
      <div class="hive-layer hive-layer--tmux">
        <strong>tmux</strong>
        <span>terminal sessions and windows</span>
      </div>
      <div class="hive-session-row">
        <div class="hive-session-card">
          <strong>session</strong>
          <span class="hive-session-item"><span class="hive-session-icon hive-session-icon--agent" aria-hidden="true"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="m4.714 15.956 4.718-2.648.079-.23-.08-.128h-.23l-.79-.048-2.695-.073-2.337-.097-2.265-.122-.57-.121-.535-.704.055-.353.48-.321.685.06 1.518.104 2.277.157 1.651.098 2.447.255h.389l.054-.158-.133-.097-.103-.098-2.356-1.596-2.55-1.688-1.336-.972-.722-.491L2 6.223l-.158-1.008.656-.722.88.06.224.061.893.686 1.906 1.476 2.49 1.833.364.304.146-.104.018-.072-.164-.274-1.354-2.446-1.445-2.49-.644-1.032-.17-.619a3 3 0 0 1-.103-.729L6.287.133 6.7 0l.995.134.42.364.619 1.415L9.735 4.14l1.555 3.03.455.898.243.832.09.255h.159V9.01l.127-1.706.237-2.095.23-2.695.08-.76.376-.91.747-.492.583.28.48.685-.067.444-.286 1.851-.558 2.903-.365 1.942h.213l.243-.242.983-1.306 1.652-2.064.728-.82.85-.904.547-.431h1.032l.759 1.129-.34 1.166-1.063 1.347-.88 1.142-1.263 1.7-.79 1.36.074.11.188-.02 2.853-.606 1.542-.28 1.84-.315.832.388.09.395-.327.807-1.967.486-2.307.462-3.436.813-.043.03.049.061 1.548.146.662.036h1.62l3.018.225.79.522.473.638-.08.485-1.213.62-1.64-.389-3.825-.91-1.31-.329h-.183v.11l1.093 1.068 2.003 1.81 2.508 2.33.127.578-.321.455-.34-.049-2.204-1.657-.85-.747-1.925-1.62h-.127v.17l.443.649 2.343 3.521.122 1.08-.17.353-.607.213-.668-.122-1.372-1.924-1.415-2.168-1.141-1.943-.14.08-.674 7.254-.316.37-.728.28-.607-.461-.322-.747.322-1.476.388-1.924.316-1.53.285-1.9.17-.632-.012-.042-.14.018-1.432 1.967-2.18 2.945-1.724 1.845-.413.164-.716-.37.066-.662.401-.589 2.386-3.036 1.439-1.882.929-1.086-.006-.158h-.055L4.138 18.56l-1.13.146-.485-.456.06-.746.231-.243 1.907-1.312Z"/></svg></span>Claude Code</span>
          <span class="hive-session-item"><span class="hive-session-icon hive-session-icon--terminal" aria-hidden="true"><svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" viewBox="0 0 24 24"><path d="M12 19h8M4 17l6-6-6-6"/></svg></span>shell</span>
        </div>
        <div class="hive-session-card">
          <strong>session</strong>
          <span class="hive-session-item"><span class="hive-session-icon hive-session-icon--agent" aria-hidden="true"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512"><path fill="currentColor" d="M196.4 185.8v-48.6c0-4.1 1.5-7.2 5.1-9.2l97.8-56.3c13.3-7.7 29.2-11.3 45.6-11.3 61.4 0 100.4 47.6 100.4 98.3 0 3.6 0 7.7-.5 11.8l-101.5-59.4c-6.1-3.6-12.3-3.6-18.4 0zm228.3 189.4V259c0-7.2-3.1-12.3-9.2-15.9L287 168.4l42-24.1c3.6-2 6.7-2 10.2 0l97.8 56.4c28.2 16.4 47.1 51.2 47.1 85 0 38.9-23 74.8-59.4 89.6zM166.2 272.8l-42-24.6c-3.6-2-5.1-5.1-5.1-9.2V126.4c0-54.8 42-96.3 98.8-96.3 21.5 0 41.5 7.2 58.4 20l-100.9 58.4c-6.1 3.6-9.2 8.7-9.2 15.9v148.5zm90.4 52.2-60.2-33.8v-71.7l60.2-33.8 60.2 33.8v71.7zm38.7 155.7c-21.5 0-41.5-7.2-58.4-20l100.9-58.4c6.1-3.6 9.2-8.7 9.2-15.9V237.9l42.5 24.6c3.6 2 5.1 5.1 5.1 9.2v112.6c0 54.8-42.5 96.3-99.3 96.3zM173.8 366.5l-97.7-56.3C47.9 293.8 29 259 29 225.2c0-39.4 23.6-74.8 59.9-89.6v116.7c0 7.2 3.1 12.3 9.2 15.9l128 74.2-42 24.1c-3.6 2-6.7 2-10.2 0zm-5.6 84c-57.9 0-100.4-43.5-100.4-97.3 0-4.1.5-8.2 1-12.3l100.9 58.4c6.1 3.6 12.3 3.6 18.4 0l128.5-74.2v48.6c0 4.1-1.5 7.2-5.1 9.2l-97.8 56.3c-13.3 7.7-29.2 11.3-45.6 11.3zm127 60.9c62 0 113.7-44 125.4-102.4 57.3-14.9 94.2-68.6 94.2-123.4 0-35.8-15.4-70.7-43-95.7 2.6-10.8 4.1-21.5 4.1-32.3 0-73.2-59.4-128-128-128-13.8 0-27.1 2-40.4 6.7-23-22.5-54.8-36.9-89.6-36.9-62 0-113.7 44-125.4 102.4-57.3 14.8-94.2 68.6-94.2 123.4 0 35.8 15.4 70.7 43 95.7-2.6 10.8-4.1 21.5-4.1 32.3 0 73.2 59.4 128 128 128 13.8 0 27.1-2 40.4-6.7 23 22.5 54.8 36.9 89.6 36.9"/></svg></span>Codex</span>
          <span class="hive-session-item"><span class="hive-session-icon hive-session-icon--terminal" aria-hidden="true"><svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" viewBox="0 0 24 24"><path d="M12 19h8M4 17l6-6-6-6"/></svg></span>tests</span>
        </div>
        <div class="hive-session-card">
          <strong>session</strong>
          <span class="hive-session-item"><span class="hive-session-icon hive-session-icon--agent" aria-hidden="true"><svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" viewBox="0 0 24 24"><path d="M9 4v16M4 7c0-1.7 1.3-3 3-3h13"/><path d="M18 20c-1.7 0-3-1.3-3-3V4"/></svg></span>Pi</span>
          <span class="hive-session-item"><span class="hive-session-icon hive-session-icon--terminal" aria-hidden="true"><svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" viewBox="0 0 24 24"><path d="M12 19h8M4 17l6-6-6-6"/></svg></span>dev server</span>
        </div>
      </div>
    </div>
  </div>
</section>

<section class="hive-preview-section">
  <h2>See What Hive Actually Is</h2>
  <p>Hive is a tmux-native command center for local agents, isolated git workspaces, project commands, and interactive terminals. It keeps your stack visible and gives Claude Code, Codex, Pi, and the rest of your tools one shared control surface.</p>

  <div class="hive-preview-terminal" aria-label="Hive terminal UI preview coming soon">
    <div class="hive-preview-terminal__nav">
      <strong>Sessions</strong><span>|</span><span>Tasks</span><span>|</span><span>Docs</span><span>|</span><span>Messages</span>
      <em><svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" viewBox="0 0 24 24" aria-hidden="true"><path d="M13 5h8"/><path d="M13 12h8"/><path d="M13 19h8"/><path d="m3 5 2 2 4-4"/><path d="m3 12 2 2 4-4"/><path d="m3 19 2 2 4-4"/></svg>2</em><b>Hive</b>
    </div>
    <div class="hive-preview-terminal__body">
      <div class="hive-preview-tree" aria-label="Hive session tree preview">
        <div class="hive-preview-tree__repo">hive <span>◆</span></div>
        <div class="hive-preview-tree__line">├─ <b>[&gt;]</b> init-command <em>#ek3p</em></div>
        <div class="hive-preview-tree__line">└─ <b>[&gt;]</b> website-homepage <em>#cwc3</em></div>
        <div class="hive-preview-tree__space"></div>
        <div class="hive-preview-tree__repo">haykot.dev</div>
        <div class="hive-preview-tree__line">└─ <b>[&gt;]</b> website <em>#1egd</em></div>
        <div class="hive-preview-tree__line hive-preview-tree__line--child">├─ <b>[&gt;]</b> pi</div>
        <div class="hive-preview-tree__line hive-preview-tree__line--child">└─ <b>[&gt;]</b> claude</div>
        <div class="hive-preview-tree__space"></div>
        <div class="hive-preview-tree__repo">infrastructure</div>
        <div class="hive-preview-tree__line">└─ <b>[&gt;]</b> infra-general <em>#d3mw</em></div>
        <div class="hive-preview-tree__space"></div>
        <div class="hive-preview-tree__repo">colonyops/hive</div>
        <div class="hive-preview-tree__line hive-preview-tree__line--active">├─ <b>[●]</b> docs-landing-page <em>#a1b2</em></div>
        <div class="hive-preview-tree__line hive-preview-tree__line--waiting">└─ <b>[!]</b> ci-fix <em>#f9e8</em></div>
      </div>
      <div class="hive-preview-pane">
        <div class="hive-preview-coming-soon">
          <span>Coming soon</span>
          <strong>Terminal preview in progress</strong>
          <em>A short capture will show sessions, live agent status, tmux previews, and coordination flows.</em>
        </div>
      </div>
    </div>
    <div class="hive-preview-terminal__footer">j/k navigate · / filter · enter select · ? help</div>
  </div>
</section>

<section class="hive-features-section">
  <div class="hive-section-heading">
    <h2>The Layer Above Your Agent Tools</h2>
    <p>Hive gives terminal agents the shared room they are missing: isolated workspaces, live visibility, shared memory, and coordination primitives.</p>
  </div>

  <div class="hive-feature-grid">
    <a class="hive-feature-card" href="getting-started/sessions/">
      <span class="hive-feature-icon" aria-hidden="true"><svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" viewBox="0 0 24 24"><path d="m12 3-8 4.5 8 4.5 8-4.5Z"/><path d="m4 12 8 4.5 8-4.5"/><path d="m4 16.5 8 4.5 8-4.5"/></svg></span>
      <strong>Workspace Management</strong>
      <p>Create and manage isolated git workspaces — full clones by default, worktrees when you prefer shared object storage.</p>
    </a>
    <a class="hive-feature-card" href="getting-started/sessions/#status-indicators">
      <span class="hive-feature-icon" aria-hidden="true"><svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" viewBox="0 0 24 24"><path d="M12 19h8M4 17l6-6-6-6"/></svg></span>
      <strong>Terminal Integration</strong>
      <p>Monitor Claude Code, Codex, Pi, shell windows, test watchers, and dev servers through tmux-backed status detection.</p>
    </a>
    <a class="hive-feature-card" href="getting-started/task-tracking/">
      <span class="hive-feature-icon" aria-hidden="true"><svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" viewBox="0 0 24 24"><path d="M13 5h8"/><path d="M13 12h8"/><path d="M13 19h8"/><path d="m3 5 2 2 4-4"/><path d="m3 12 2 2 4-4"/><path d="m3 19 2 2 4-4"/></svg></span>
      <strong>Task Tracking</strong>
      <p>Use built-in epics, tasks, blockers, comments, and assignment to coordinate work across agents.</p>
    </a>
    <a class="hive-feature-card" href="getting-started/context/">
      <span class="hive-feature-icon" aria-hidden="true"><svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" viewBox="0 0 24 24"><path d="M12 10v6"/><path d="M9 13h6"/><path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/></svg></span>
      <strong>Shared Context</strong>
      <p>Keep plans, research, notes, and handoffs in a repo-scoped `.hive` context directory agents can share.</p>
    </a>
    <a class="hive-feature-card" href="getting-started/messaging/">
      <span class="hive-feature-icon" aria-hidden="true"><svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" viewBox="0 0 24 24"><path d="m22 7-8.991 5.727a2 2 0 0 1-2.009 0L2 7"/><rect x="2" y="4" width="20" height="16" rx="2"/></svg></span>
      <strong>Inter-agent Messaging</strong>
      <p>Send direct or broadcast messages between sessions using Hive's pub/sub inboxes.</p>
    </a>
    <a class="hive-feature-card" href="configuration/commands/">
      <span class="hive-feature-icon" aria-hidden="true"><svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" viewBox="0 0 24 24"><path d="M15 6v12a3 3 0 1 0 3-3H6a3 3 0 1 0 3 3V6a3 3 0 1 0-3 3h12a3 3 0 1 0-3-3"/></svg></span>
      <strong>Custom Workflows</strong>
      <p>Bind keys and commands to your own scripts, review flows, dev servers, test runners, and agent tools.</p>
    </a>
  </div>
</section>

## Start Building With Hive

<div class="hive-cta">
  <div>
    <h2>Install Hive and give your terminal agents a shared room to work in.</h2>
    <p>Prefer a deeper walkthrough? Start with the getting started guide.</p>
  </div>
  <div class="hive-cta__actions">
    <a class="md-button md-button--primary" href="getting-started/">Getting Started</a>
    <a class="md-button" href="configuration/">Configuration</a>
  </div>
</div>

---

<small>LLM-friendly: [llms.txt](../llms.txt) | [llms-full.txt](../llms-full.txt)</small>
