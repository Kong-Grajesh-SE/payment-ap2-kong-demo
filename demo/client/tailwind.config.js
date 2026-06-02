/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{js,ts,jsx,tsx}"],
  theme: {
    extend: {
      colors: {
        /* Kong Design System v1.3 — "Electric Lime on Dark Green" */
        lime: "rgb(204, 255, 0)",
        "lime-hover": "rgb(184, 230, 0)",
        "dark-green": "rgb(0, 15, 6)",
        black: "rgb(0, 0, 0)",
        kong: {
          lime: "rgb(204, 255, 0)",
          dark: "rgb(0, 15, 6)",
          black: "rgb(0, 0, 0)",
        },
        surface: {
          0: "rgb(0, 15, 6)",      /* page bg */
          100: "rgb(0, 0, 0)",     /* elevated */
          200: "rgb(13, 16, 5)",   /* card */
          300: "rgb(48, 53, 47)",  /* tertiary */
        },
        fg: {
          0: "rgb(255, 255, 255)",
          1: "rgb(186, 193, 184)",
          2: "rgb(174, 181, 167)",
          3: "rgb(161, 166, 159)",
        },
        border: {
          subtle: "rgb(49, 57, 25)",
          DEFAULT: "rgb(74, 77, 73)",
          strong: "rgb(161, 166, 159)",
        },
      },
      fontFamily: {
        sans: ['"Funnel Sans"', '"Inter"', "system-ui", "-apple-system", "sans-serif"],
        mono: ['"Roboto Mono"', '"JetBrains Mono"', "ui-monospace", "Menlo", "monospace"],
      },
      borderRadius: {
        xs: "2px",
        sm: "5px",
        md: "10px",
        lg: "16px",
        pill: "10000px",
      },
      animation: {
        "pulse-slow": "pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite",
        "slide-up": "slideUp 0.26s cubic-bezier(.2,.8,.2,1)",
        "fade-in": "fadeIn 0.26s cubic-bezier(.2,.8,.2,1)",
        "glow": "glowPulse 2s ease-in-out infinite alternate",
      },
      keyframes: {
        slideUp: {
          "0%": { transform: "translateY(10px)", opacity: "0" },
          "100%": { transform: "translateY(0)", opacity: "1" },
        },
        fadeIn: {
          "0%": { opacity: "0" },
          "100%": { opacity: "1" },
        },
        glowPulse: {
          "0%": { boxShadow: "0 0 40px rgba(204,255,0,0.2)" },
          "100%": { boxShadow: "0 0 80px rgba(204,255,0,0.45)" },
        },
      },
    },
  },
  plugins: [],
};
