import { build, emptyDir } from "https://deno.land/x/dnt/mod.ts";

await emptyDir("./dist");

await build({
  entryPoints: ["client/src/index.ts"],
  outDir: "client/dist",
  shims: {},
  rootTestDir: "client/test",
  test: false,
  package: {
    "name": "data-plane-gateway",
    "version": "0.0.0",
    "description": "Client library for interacting with the Flow / Gazette gateway.",
    "homepage": "https://github.com/estuary/data-plane-gateway",
    "author": "Alex Burkhart",
    "license": "MIT",
    "repository": {
      "type": "git",
      "url": "github.com/estuary/data-plane-gateway",
      "directory": "client"
    },
    "publishConfig": {
      "registry":"https://npm.pkg.github.com"
    },
    "dependencies": {
    }
  }
});
