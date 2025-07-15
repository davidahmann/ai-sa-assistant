const js = require('@eslint/js');
const globals = require('globals');

module.exports = [
  {
    // Apply to all JavaScript files in the frontend
    files: ['cmd/webui/static/**/*.js'],
    languageOptions: {
      ecmaVersion: 2022,
      sourceType: 'script', // Service workers and regular scripts use 'script' not 'module'
      globals: {
        ...globals.browser,
        ...globals.serviceworker,
        // Frontend-specific globals
        mermaid: 'readonly',
        Prism: 'readonly',
        marked: 'readonly',
        DOMPurify: 'readonly',
        // Service Worker globals
        self: 'readonly',
        clients: 'readonly',
        caches: 'readonly',
        registration: 'readonly',
        skipWaiting: 'readonly',
        // Custom app globals
        chatApp: 'writable'
      }
    },
    rules: {
      // Extend recommended rules
      ...js.configs.recommended.rules,

      // Code Quality
      'no-unused-vars': ['error', {
        argsIgnorePattern: '^_',
        varsIgnorePattern: '^_',
        caughtErrorsIgnorePattern: '^_'
      }],
      'no-console': ['warn', {
        allow: ['warn', 'error', 'log']
      }],
      'no-debugger': 'error',
      'no-alert': 'warn',
      'no-eval': 'error',
      'no-implied-eval': 'error',

      // Best Practices
      'eqeqeq': ['error', 'always'],
      'curly': ['error', 'all'],
      'dot-notation': 'error',
      'no-multi-spaces': 'error',
      'no-trailing-spaces': 'error',
      'no-multiple-empty-lines': ['error', { max: 2, maxEOF: 1 }],

      // Style (will be handled by Prettier but good to have as backup)
      'indent': ['error', 4, { SwitchCase: 1 }],
      'quotes': ['error', 'single', { avoidEscape: true }],
      'semi': ['error', 'always'],
      'comma-dangle': ['error', 'never'],
      'brace-style': ['error', '1tbs', { allowSingleLine: true }],

      // ES6+
      'prefer-const': 'error',
      'no-var': 'error',
      'prefer-arrow-callback': 'error',
      'arrow-spacing': 'error',
      'prefer-template': 'error',

      // Functions
      'no-unused-expressions': 'error',
      'consistent-return': 'error',
      'default-case': 'error',

      // Browser/DOM specific
      'no-global-assign': 'error',
      'no-implicit-globals': 'error'
    }
  },
  {
    // Special configuration for service worker
    files: ['cmd/webui/static/sw.js'],
    languageOptions: {
      globals: {
        ...globals.serviceworker,
        console: 'readonly'
      }
    },
    rules: {
      // Service workers can use console more liberally
      'no-console': 'off'
    }
  },
  {
    // Ignore minified vendor libraries
    ignores: [
      'cmd/webui/static/libs/**/*.min.js',
      'node_modules/**/*'
    ]
  }
];
