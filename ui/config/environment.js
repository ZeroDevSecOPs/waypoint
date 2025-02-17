'use strict';

module.exports = function (environment) {
  let ENV = {
    apiAddress: '',
    modulePrefix: 'waypoint',
    environment,
    rootURL: '/',
    locationType: 'auto',
    EmberENV: {
      FEATURES: {
        // Here you can enable experimental features on an ember canary build
        // e.g. EMBER_NATIVE_DECORATOR_SUPPORT: true
      },
      EXTEND_PROTOTYPES: {
        // Prevent Ember Data from overriding Date.parse.
        Date: false,
      },
    },

    APP: {
      // Here you can pass flags/options to your application instance
      // when it is created
    },

    // Ember-cli-flash addon settings
    flashMessageDefaults: {
      destroyOnClick: false,
      timeout: 5000,
      extendedTimeout: 700,
      type: 'info',
      types: ['error', 'success', 'info', 'warning'],
      injectionFactories: [],
    },

    // Ember-toggle addon settings
    'ember-toggle': {
      includedThemes: ['light'],
      defaultTheme: 'light',
    },
  };

  if (environment === 'development') {
    // ENV.APP.LOG_RESOLVER = true;
    // ENV.APP.LOG_ACTIVE_GENERATION = true;
    // ENV.APP.LOG_TRANSITIONS = true;
    // ENV.APP.LOG_TRANSITIONS_INTERNAL = true;
    // ENV.APP.LOG_VIEW_LOOKUPS = true;
    ENV['ember-cli-mirage'] = {
      // Allows for `ember serve no-mirage`
      enabled: process.argv.includes('no-mirage') ? false : true,
    };

    if (process.argv.includes('local')) {
      // Default Docker server address
      ENV.apiAddress = 'https://localhost:9702';
      ENV['ember-cli-mirage'] = {
        enabled: false,
      };
    }
  }

  if (environment === 'test') {
    // Testem prefers this...
    ENV.locationType = 'none';

    // keep test console output quieter
    ENV.APP.LOG_ACTIVE_GENERATION = false;
    ENV.APP.LOG_VIEW_LOOKUPS = false;

    ENV.APP.rootElement = '#ember-testing';
    ENV.APP.autoboot = false;
  }

  if (environment === 'production') {
    // here you can enable a production-specific feature
  }

  return ENV;
};
