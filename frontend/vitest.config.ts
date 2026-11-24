import { defineConfig, mergeConfig } from "vitest/config"
import viteConfig from "./vite.config.ts"

export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      environment: "jsdom",
      // forks, not threads: process.env.TZ only takes effect in a real process
      pool: "forks",
      setupFiles: ["./src/test/setup.ts"],
      include: ["src/**/*.test.{ts,tsx}"],
      css: false,
      restoreMocks: true,
      coverage: {
        provider: "v8",
        include: ["src/**/*.{ts,tsx}"],
        exclude: ["src/**/*.test.{ts,tsx}", "src/test/**", "src/components/ui/**", "src/**/*.d.ts", "src/main.tsx"],
        reporter: ["text-summary", "html"],
        reportsDirectory: "coverage",
      },
    },
  }),
)
