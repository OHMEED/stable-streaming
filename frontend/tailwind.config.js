/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  darkMode: 'class',
  theme: {
    extend: {
      fontFamily: {
        sans: ['Outfit', 'Inter', 'sans-serif'],
      },
      colors: {
        // Dark mode backgrounds
        dark: {
          900: '#1e1e1e', // Primary dark background
          800: '#221D23', // Secondary dark
          700: '#323031', // Tertiary dark
          600: '#3d3a3e',
          500: '#555255',
          400: '#7a777b',
          300: '#a19ea2',
          200: '#c8c6c9',
          100: '#e4e3e5',
        },
        // Light mode background
        light: {
          50: '#FFFFFF',
          100: '#F7EBE8', // Primary light background
          200: '#EDE1DE',
          300: '#DED0CC',
          400: '#C5B5B0',
          500: '#9A8A85',
          600: '#6F5F5A',
          700: '#453530',
          800: '#2a1f1a',
          900: '#1a110d',
        },
        // Accent colors
        accent: {
          red: '#f54329', // Primary accent (logo color)
          'red-light': '#ff6b55',
          'red-dark': '#c93620',
          cyan: '#53F4FF', // Eye-popping blue
          'cyan-light': '#A5F0F4', // Softer cyan
          'cyan-dark': '#3bc9d3',
        },
        // Keep some gray utilities
        gray: {
          900: '#1e1e1e',
          800: '#2a2a2a',
          700: '#3a3a3a',
          600: '#4a4a4a',
          500: '#6a6a6a',
          400: '#8a8a8a',
          300: '#b0b0b0',
          200: '#d0d0d0',
          100: '#f0f0f0',
        },
        // Theme-aware semantic colors
        primary: {
          50: '#fff5f3',
          100: '#ffe8e4',
          200: '#ffd5cd',
          300: '#ffb5a8',
          400: '#ff8975',
          500: '#f54329', // Main accent
          600: '#e63620',
          700: '#c22919',
          800: '#a12518',
          900: '#85241a',
        },
      },
      animation: {
        'fade-in': 'fadeIn 0.5s ease-out',
        'slide-up': 'slideUp 0.5s ease-out',
        'pulse-glow': 'pulseGlow 2s infinite',
        'slide-in-right': 'slideInRight 0.3s ease-out',
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' },
        },
        slideUp: {
          '0%': { transform: 'translateY(20px)', opacity: '0' },
          '100%': { transform: 'translateY(0)', opacity: '1' },
        },
        pulseGlow: {
          '0%, 100%': { opacity: '1', transform: 'scale(1)' },
          '50%': { opacity: '0.8', transform: 'scale(1.05)' },
        },
        slideInRight: {
          '0%': { transform: 'translateX(100%)', opacity: '0' },
          '100%': { transform: 'translateX(0)', opacity: '1' },
        },
      }
    },
  },
  plugins: [],
}
