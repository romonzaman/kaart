// Dynamic layer over app.json.
//
// It exists for one reason: the web build has to be able to live under a
// subpath. Kaart is served from https://agent.mkzaman.com/kaartd/, not from a
// domain root, and both the asset URLs Expo writes into index.html and
// expo-router's own route matching need to know that prefix at export time.
//
// Keeping it here rather than in app.json means `npx expo start --web` still
// serves from / during development, where no prefix exists.
//
//   EXPO_BASE_URL      path the app is served under, no trailing slash ('/kaartd')
//   EXPO_PUBLIC_API_URL prefix the API client puts before /api/v1 and /healthz
//
// Both are set by `make web`.

module.exports = ({ config }) => {
  const baseUrl = (process.env.EXPO_BASE_URL ?? '').replace(/\/+$/, '');

  return {
    ...config,
    experiments: {
      ...config.experiments,
      // Empty means "serve from the root", which is what dev wants.
      ...(baseUrl ? { baseUrl } : {}),
    },
  };
};
