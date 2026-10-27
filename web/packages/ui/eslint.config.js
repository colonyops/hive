// @ts-check
import js from '@eslint/js'
import prettier from 'eslint-config-prettier/flat'
import vue from 'eslint-plugin-vue'
import { defineConfig } from 'eslint/config'
import tseslint from 'typescript-eslint'

// Module-level reactive state is shared state, and the host app owns shared
// state. Only the initializer of a top-level declaration is matched: a `ref`
// inside a function body is per-call and allowed.
/** @param {string} message */
function moduleStateSelectors(message) {
  const call = 'CallExpression[callee.name=/^(ref|shallowRef|reactive|shallowReactive|useStorage)$/]'
  return ['Program > VariableDeclaration', 'Program > ExportNamedDeclaration > VariableDeclaration'].flatMap(
    (declaration) => [
      { selector: `${declaration} > VariableDeclarator > ${call}`, message },
      { selector: `${declaration} > VariableDeclarator > CallExpression > ${call}`, message },
    ],
  )
}

const stateOutsideApp = moduleStateSelectors(
  '@hive/ui holds no shared state. Keep module-level reactive state in the host app.',
)
// A component reaches the DOM, timers, and storage through VueUse, which ties
// each one to the component's lifetime (cmd/desktop/AGENTS.md).
const rawBrowserApisInComponents = [
  {
    selector: 'CallExpression[callee.property.name="addEventListener"]',
    message: 'Use useEventListener (or onKeyStroke, onClickOutside) from @vueuse/core.',
  },
  { selector: 'NewExpression[callee.name="ResizeObserver"]', message: 'Use useResizeObserver from @vueuse/core.' },
  { selector: 'CallExpression[callee.name="setInterval"]', message: 'Use useIntervalFn from @vueuse/core.' },
]

export default defineConfig(
  js.configs.recommended,
  tseslint.configs.recommendedTypeChecked,
  vue.configs['flat/essential'],

  {
    languageOptions: {
      parserOptions: {
        parser: tseslint.parser,
        projectService: { allowDefaultProject: ['*.config.ts', 'eslint.config.js'] },
        tsconfigRootDir: import.meta.dirname,
        extraFileExtensions: ['.vue'],
      },
    },
    linterOptions: { reportUnusedDisableDirectives: 'error' },
    rules: {
      '@typescript-eslint/no-unused-vars': [
        'error',
        { argsIgnorePattern: '^_', varsIgnorePattern: '^_', caughtErrors: 'none', ignoreRestSiblings: true },
      ],
      '@typescript-eslint/switch-exhaustiveness-check': [
        'error',
        { considerDefaultExhaustiveForUnions: true, requireDefaultForNonUnion: false },
      ],
      '@typescript-eslint/no-deprecated': 'error',
      '@typescript-eslint/no-unnecessary-type-assertion': 'error',
      '@typescript-eslint/no-import-type-side-effects': 'error',
      '@typescript-eslint/use-unknown-in-catch-callback-variable': 'error',
      // TypeScript already resolves every identifier; this rule only knows ESLint's globals list.
      'no-undef': 'off',
      'no-new-func': 'error',
      'no-console': 'error',
      eqeqeq: ['error', 'always', { null: 'ignore' }],
      'no-fallthrough': 'error',

      'vue/no-ref-object-reactivity-loss': 'error',
      'vue/no-setup-props-reactivity-loss': 'error',
      'vue/no-unused-refs': 'error',
      'vue/no-unused-emit-declarations': 'error',
      'vue/require-explicit-emits': 'error',
      'vue/no-v-html': 'error',
      'vue/no-template-target-blank': 'error',
      'vue/no-useless-template-attributes': 'error',
      'vue/no-unused-properties': ['error', { groups: ['props', 'setup'] }],
      'vue/block-lang': ['error', { script: { lang: 'ts' } }],
      'vue/multi-word-component-names': 'off',
    },
  },

  {
    files: ['src/**/*.{ts,vue}'],
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            { group: ['@wailsio/*'], message: '@hive/ui runs outside Wails. Inject through HiveUiPlatform.' },
            { group: ['**/bindings/**'], message: '@hive/ui cannot import Wails bindings.' },
            { group: ['**/cmd/**', '**/frontend/**'], message: '@hive/ui cannot import an app.' },
          ],
        },
      ],
    },
  },

  // Flat config replaces a rule's options rather than merging them, so every
  // file must get its whole no-restricted-syntax list from one config object.
  {
    files: ['src/**/*.vue'],
    rules: {
      'no-restricted-syntax': ['error', ...rawBrowserApisInComponents],
      'no-restricted-globals': ['error', { name: 'localStorage', message: 'Use useStorage from @vueuse/core.' }],
      'no-restricted-properties': [
        'error',
        { object: 'window', property: 'localStorage', message: 'Use useStorage from @vueuse/core.' },
      ],
      // The rule has no custom message: use the text, radius, and shadow
      // tokens in src/components/ui/README.md ("Tokens") instead.
      'vue/no-restricted-class': [
        'error',
        '/^([\\w-]+:)*text-\\[\\d/',
        '/^([\\w-]+:)*rounded(-[a-z]{1,2})?-\\[/',
        '/^([\\w-]+:)*shadow-\\[/',
      ],
    },
  },
  {
    files: ['src/**/*.ts'],
    ignores: ['**/*.spec.ts', 'src/test-utils/**', 'src/test-setup.ts'],
    rules: { 'no-restricted-syntax': ['error', ...stateOutsideApp] },
  },

  {
    files: ['**/*.spec.ts', 'src/test-setup.ts', 'src/test-utils/**'],
    rules: {
      '@typescript-eslint/no-unsafe-assignment': 'off',
      '@typescript-eslint/no-unsafe-member-access': 'off',
      '@typescript-eslint/no-unsafe-call': 'off',
      '@typescript-eslint/no-unsafe-argument': 'off',
      '@typescript-eslint/unbound-method': 'off',
    },
  },

  prettier,
)
