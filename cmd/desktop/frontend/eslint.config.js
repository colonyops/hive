// @ts-check
import js from '@eslint/js'
import prettier from 'eslint-config-prettier/flat'
import vue from 'eslint-plugin-vue'
import { defineConfig, globalIgnores } from 'eslint/config'
import tseslint from 'typescript-eslint'

// Module-level reactive state is shared state, and shared state is a store
// (src/stores/README.md). Only the initializer of a top-level declaration is
// matched: a `ref` inside a function body is per-call and allowed.
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

const stateOutsideStores = moduleStateSelectors(
  'Module-level reactive state is shared state. Move it into a store under src/stores/ (src/stores/README.md).',
)
const stateOutsideSetup = moduleStateSelectors(
  "Create a store's state inside its defineStore setup, so resetStores() can reset it.",
)
const rawWailsSubscription = {
  selector: 'MemberExpression[object.name="Events"][property.name="On"]',
  message: 'Subscribe through useWailsEvent so the subscription ends with its scope.',
}

export default defineConfig(
  globalIgnores(['bindings/', 'dist/']),

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
      'no-console': ['error', { allow: ['warn', 'error'] }],
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

  // Flat config replaces a rule's options rather than merging them, so every
  // file must get its whole no-restricted-syntax list from one config object.
  {
    files: ['src/**/*.{ts,vue}'],
    rules: { 'no-restricted-syntax': ['error', rawWailsSubscription] },
  },
  {
    files: ['src/**/*.ts'],
    ignores: ['src/stores/**', '**/*.spec.ts', 'src/test-utils/**', 'src/test-setup.ts'],
    rules: { 'no-restricted-syntax': ['error', ...stateOutsideStores, rawWailsSubscription] },
  },
  {
    files: ['src/stores/**/*.ts'],
    ignores: ['**/*.spec.ts'],
    rules: { 'no-restricted-syntax': ['error', ...stateOutsideSetup, rawWailsSubscription] },
  },
  {
    files: ['src/composables/useWailsEvent.ts'],
    rules: { 'no-restricted-syntax': ['error', ...stateOutsideStores] },
  },

  {
    // Node execution lives in Go (ADR flow-engine-in-go); the frontend only
    // owns each node type's editor. A runtime module here would be a second,
    // uncalled implementation of a node's semantics that drifts from the real one.
    files: ['src/pipeline/nodes/**'],
    rules: {
      'no-restricted-imports': [
        'error',
        { patterns: [{ group: ['**/runtime', '**/runtime.ts'], message: 'Node execution lives in Go.' }] },
      ],
    },
  },
  {
    files: ['src/pipeline/nodes/*/runtime.ts'],
    rules: {
      'no-restricted-syntax': [
        'error',
        { selector: 'Program', message: 'Node execution lives in Go (cmd/desktop/internal/app/runtime).' },
      ],
    },
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
