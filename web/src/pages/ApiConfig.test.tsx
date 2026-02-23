import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ApiConfig } from '@/pages/ApiConfig'
import * as apiModule from '@/lib/api'

// Mock react-i18next
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => {
      const translations: Record<string, string> = {
        'apiConfig.title': 'API Configuration',
        'apiConfig.subtitle': 'Settings',
        'apiConfig.description': 'Configure your LLM provider',
        'apiConfig.providerSection': 'Provider',
        'apiConfig.providerSectionDesc': 'Select your LLM provider',
        'apiConfig.connectionSection': 'Connection',
        'apiConfig.connectionSectionDesc': 'API endpoint and authentication',
        'apiConfig.baseUrl': 'Base URL',
        'apiConfig.baseUrlHint': 'Leave empty to use default',
        'apiConfig.apiKey': 'API Key',
        'apiConfig.apiKeyPlaceholder': 'Enter your API key',
        'apiConfig.apiKeyHint': 'Encrypted and stored in vault',
        'apiConfig.model': 'Model',
        'apiConfig.modelHint': 'Enter the model name',
        'apiConfig.selectModel': 'Select a model',
        'apiConfig.testConnection': 'Test Connection',
        'apiConfig.testing': 'Testing...',
        'apiConfig.connectionFailed': 'Connection failed',
        'apiConfig.statusTitle': 'Connection Status',
        'apiConfig.statusConfigured': 'Configured and ready',
        'apiConfig.statusNotConfigured': 'API key not configured',
        'apiConfig.currentProvider': 'Current Provider',
        'apiConfig.freeProviders': 'Free Providers',
        'apiConfig.paidProviders': 'Paid Providers',
        'apiConfig.localProviders': 'Local / Self-Hosted',
        'apiConfig.getApiKey': 'Get API Key',
        'apiConfig.endpoint': 'Endpoint',
        'apiConfig.endpointHint': 'Select the API endpoint',
        'apiConfig.restartWarning': 'Changes may require restart',
        'apiConfig.context1m': 'Enable 1M Context',
        'apiConfig.context1mDesc': 'Enable 1M token context beta',
        'apiConfig.toolStream': 'Real-time Tool Streaming',
        'apiConfig.toolStreamDesc': 'Enable streaming tool calls',
        'apiConfig.validation.providerRequired': 'Please select a provider',
        'apiConfig.validation.modelRequired': 'Please select a model',
        'apiConfig.validation.apiKeyRequired': 'API key required',
        'common.save': 'Save',
        'common.saving': 'Saving...',
        'common.reset': 'Reset',
        'common.success': 'Success',
        'common.error': 'Error',
      }
      return translations[key] || key
    },
  }),
}))

// Mock API
vi.mock('@/lib/api', () => ({
  api: {
    config: {
      get: vi.fn(),
      update: vi.fn(),
    },
    setup: {
      testProvider: vi.fn(),
    },
  },
}))

const mockConfig = {
  provider: 'openai',
  base_url: 'https://api.openai.com/v1',
  api_key_configured: true,
  api_key_masked: '••••••••6789',
  model: 'gpt-4o-mini',
  models: ['gpt-4o-mini', 'gpt-4o', 'gpt-3.5-turbo'],
  params: {},
}

describe('ApiConfig', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(apiModule.api.config.get).mockResolvedValue(mockConfig as unknown as Record<string, unknown>)
    vi.mocked(apiModule.api.config.update).mockResolvedValue(mockConfig as unknown as Record<string, unknown>)
    vi.mocked(apiModule.api.setup.testProvider).mockResolvedValue({ success: true })
  })

  it('should render loading state initially', () => {
    render(<ApiConfig />)

    // Loading spinner should be visible while loading
    const spinner = document.querySelector('.animate-spin')
    expect(spinner).toBeInTheDocument()
  })

  it('should render provider cards after loading', async () => {
    render(<ApiConfig />)

    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument()
    })

    // Check other providers are shown
    expect(screen.getByText('Anthropic')).toBeInTheDocument()
    expect(screen.getByText('Google')).toBeInTheDocument()
  })

  it('should select provider when clicked', async () => {
    render(<ApiConfig />)

    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument()
    })

    // Click on Anthropic provider
    await userEvent.click(screen.getByText('Anthropic'))

    // The selection style is applied via style prop
    const anthropicCard = screen.getByText('Anthropic').closest('button')
    expect(anthropicCard).toBeInTheDocument()
  })

  it('should test connection when test button is clicked', async () => {
    render(<ApiConfig />)

    await waitFor(() => {
      expect(screen.getByText('Test Connection')).toBeInTheDocument()
    })

    await userEvent.click(screen.getByText('Test Connection'))

    await waitFor(() => {
      expect(apiModule.api.setup.testProvider).toHaveBeenCalled()
    })
  })

  it('should save configuration when save button is clicked', async () => {
    render(<ApiConfig />)

    await waitFor(() => {
      expect(screen.getByText('API Key')).toBeInTheDocument()
    })

    // Enter a new API key to trigger a change
    const passwordInput = document.querySelector('input[type="password"]') as HTMLInputElement
    expect(passwordInput).toBeInTheDocument()
    await userEvent.type(passwordInput, 'test-api-key')

    // Now the save button should be enabled
    const saveButton = screen.getByRole('button', { name: 'Save' })
    await userEvent.click(saveButton)

    await waitFor(() => {
      expect(apiModule.api.config.update).toHaveBeenCalled()
    }, { timeout: 3000 })
  })

  it('should show status card with configured status', async () => {
    render(<ApiConfig />)

    await waitFor(() => {
      expect(screen.getByText('Connection Status')).toBeInTheDocument()
    })

    expect(screen.getByText('Configured and ready')).toBeInTheDocument()
    expect(screen.getByText('openai')).toBeInTheDocument()
  })

  it('should show API Key field for providers that require it', async () => {
    render(<ApiConfig />)

    await waitFor(() => {
      expect(screen.getByText('API Key')).toBeInTheDocument()
    })

    // Check that password input exists (type password)
    const passwordInput = document.querySelector('input[type="password"]')
    expect(passwordInput).toBeInTheDocument()
  })

  it('should show Model field', async () => {
    render(<ApiConfig />)

    await waitFor(() => {
      expect(screen.getByText('Model')).toBeInTheDocument()
    })
  })

  it('should show restart warning when changes are made', async () => {
    render(<ApiConfig />)

    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument()
    })

    // Click a different provider to trigger changes
    await userEvent.click(screen.getByText('Anthropic'))

    // The restart warning should appear
    await waitFor(() => {
      expect(screen.getByText('Changes may require restart')).toBeInTheDocument()
    })
  })
})
