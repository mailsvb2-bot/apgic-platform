const path = require("node:path");
const {getDefaultConfig, mergeConfig} = require("@react-native/metro-config");

const repositoryRoot = path.resolve(__dirname, "../..");

const config = {
  // APGIC is a monorepo. The native bundle consumes the canonical generated
  // contract from packages/contracts, so Metro must explicitly watch the
  // repository root instead of silently limiting resolution to apps/mobile.
  watchFolders: [repositoryRoot],
};

module.exports = mergeConfig(getDefaultConfig(__dirname), config);
