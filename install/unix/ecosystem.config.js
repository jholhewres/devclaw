/**
 * PM2 Ecosystem Configuration for DevClaw
 *
 * Usage:
 *   pm2 start ecosystem.config.js
 *   pm2 save
 *   pm2 startup  # follow the printed command
 *
 * Environment Variables:
 *   PORT              - Server port (default: 8090)
 *   DEVCLAW_STATE_DIR - State directory (default: /opt/devclaw)
 *   NODE_ENV          - Environment (production/development)
 */

const path = require('path');

// Allow customization via environment
const installDir = process.env.DEVCLAW_INSTALL_DIR || '/opt/devclaw';
const port = process.env.PORT || '8090';

module.exports = {
  apps: [{
    name: 'devclaw',
    script: path.join(installDir, 'devclaw'),
    args: 'serve',
    cwd: installDir,

    // Process management
    instances: 1,
    autorestart: true,
    watch: false,
    max_memory_restart: '1G',
    kill_timeout: 5000,
    wait_ready: true,
    listen_timeout: 10000,

    // Logging
    time: true,
    log_date_format: 'YYYY-MM-DD HH:mm:ss Z',
    error_file: path.join(installDir, 'logs', 'error.log'),
    out_file: path.join(installDir, 'logs', 'out.log'),
    merge_logs: true,

    // Environment
    env: {
      NODE_ENV: 'production',
      DEVCLAW_STATE_DIR: installDir,
      PORT: port
    },

    // Development overrides (pm2 start ecosystem.config.js --env development)
    env_development: {
      NODE_ENV: 'development',
      DEVCLAW_STATE_DIR: installDir,
      PORT: port
    }
  }]
};
