/**
 * Example CONTROL plugin.
 *
 * Exports a `plugin` object conforming to the PluginModule interface.
 * Tools declared here are automatically registered with the agent tool registry
 * when the plugin is loaded.
 */

const echoTool = {
  name: 'echo',
  description: 'Echoes the input text back to the user. Use when the user explicitly asks to repeat or echo a message.',
  paramsSchema: null,
  /** @param {Record<string, unknown>} params */
  async execute(params) {
    const text = String(params.text ?? params.message ?? '');
    return { text, echoed: true };
  },
};

export const plugin = {
  tools: [echoTool],

  async onStartup() {
    // This hook runs once after all plugins are loaded.
    // Use it for one-time initialization (e.g., connect to a database).
    console.log('[EXAMPLE-PLUGIN] Loaded successfully');
  },
};
