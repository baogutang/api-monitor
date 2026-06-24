/** @type {import('tailwindcss').Config} */
export default {
  darkMode: ['class', '[data-theme-mode="dark"]'],
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['var(--font-sans)'],
        mono: ['var(--font-mono)'],
      },
      colors: {
        bg: {
          void: 'var(--bg-void)',
          panel: 'var(--bg-panel)',
          surface: 'var(--bg-surface)',
          elevated: 'var(--bg-elevated)',
        },
        brand: {
          DEFAULT: 'var(--brand)',
          bright: 'var(--brand-bright)',
          dim: 'var(--brand-dim)',
        },
        text: {
          1: 'var(--text-1)',
          2: 'var(--text-2)',
          3: 'var(--text-3)',
          4: 'var(--text-4)',
        },
        border: {
          DEFAULT: 'var(--border)',
          strong: 'var(--border-strong)',
        },
        ok: 'var(--ok)',
        warn: 'var(--warn)',
        crit: 'var(--crit)',
      },
      borderRadius: {
        sm: 'var(--radius-sm)',
        md: 'var(--radius-md)',
        lg: 'var(--radius-lg)',
        xl: 'var(--radius-xl)',
      },
      boxShadow: {
        card: 'var(--shadow-elevated)',
        glow: 'var(--shadow-glow)',
      },
    },
  },
  plugins: [],
}
