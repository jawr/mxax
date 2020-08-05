const { colors } = require('tailwindcss/defaultTheme')

module.exports = {
  purge: {
    enabled: true,
    content: [
        '../**/*.html',
    ],
  },
  theme: {
      colors: {
          black: colors.black,
          white: colors.white,
          gray: colors.gray,
          red: colors.red,
          green: colors.green,
          blue: colors.blue,
          orange: colors.orange,
    },
    extend: {},
  },
  variants: {},
  plugins: [],
}
