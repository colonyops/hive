// @ts-check
import js from '@eslint/js'
import prettier from 'eslint-config-prettier/flat'
import vue from 'eslint-plugin-vue'
import { defineConfig, globalIgnores } from 'eslint/config'
import tseslint from 'typescript-eslint'

// Module-level reactive state is shared across every mounted page and test.
// Only the initializer of a top-level declaration is matched: a `ref`
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
  'Module-level reactive state is shared. Create it inside a composable or provide it from the layout.',
)
// A component reaches the DOM, timers, and storage through VueUse, which ties
// each one to the component's lifetime (cmd/desktop/AGENTS.md).
const rawBrowserApisInComponents = [
  {
    selector: 'CallExpression[callee.property.name="addEventListener"]',
    message: 'Use useEventListener (or onKeyStroke, onClickOutside) from @vueuse/core.',
  },
]
// useStorage survives blocked storage and keeps every reader of a key in step.
const rawStorage = {
  'no-restricted-globals': ['error', { name: 'localStorage', message: 'Use useStorage from @vueuse/core.' }],
  'no-restricted-properties': [
    'error',
    { object: 'window', property: 'localStorage', message: 'Use useStorage from @vueuse/core.' },
  ],
}

export default defineConfig(
  globalIgnores(['dist/']),

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
      // TypeScript already resolves every identifier; this rule only knows ESLint's globals list.
      'no-undef': 'off',
      'no-console': 'error',
      eqeqeq: ['error', 'always', { null: 'ignore' }],

      'vue/no-ref-object-reactivity-loss': 'error',
      'vue/no-setup-props-reactivity-loss': 'error',
      'vue/no-unused-refs': 'error',
      'vue/no-unused-emit-declarations': 'error',
      'vue/require-explicit-emits': 'error',
      'vue/no-v-html': 'error',
      'vue/no-unused-properties': ['error', { groups: ['props', 'setup'] }],
      'vue/block-lang': ['error', { script: { lang: 'ts' } }],
      'vue/multi-word-component-names': 'off',
    },
  },

  // Flat config replaces a rule's options rather than merging them, so every
  // file must get its whole no-restricted-syntax list from one config object.
  {
    files: ['src/**/*.vue'],
    rules: {
      'no-restricted-syntax': ['error', ...rawBrowserApisInComponents],
      ...rawStorage,
      // The rule has no custom message: use the text, radius, and shadow
      // tokens in web/packages/ui/src/components/ui/README.md ("Tokens") instead.
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
    rules: { 'no-restricted-syntax': ['error', ...stateOutsideApp], ...rawStorage },
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
