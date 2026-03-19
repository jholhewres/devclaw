import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { ApiConfig } from '@/pages/ApiConfig'
import * as apiModule from '@/lib/api'

// Mock react-i18next
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => {
      const translations: Record<string, string> = {
        'config.pageTitle': 'Configuration',
        'config.pageDescription': 'Configure your LLM provider',
        'config.providerModel': 'Provider & Model',
        'config.providerDesc': 'Select your LLM provider and model',
        'config.seeMore': 'See More',
        'config.visionTitle': 'Vision',
        'config.visionDesc': 'Image analysis settings',
        'config.visionModel': 'Vision Model',
        'config.visionModelHint': 'Uses main model if empty',
        'config.visionModelPlaceholder': 'e.g. gpt-4o',
        'config.visionQuality': 'Quality',
        'config.visionQualityAuto': 'Auto',
        'config.visionQualityLow': 'Low',
        'config.visionQualityHigh': 'High',
        'config.transcriptionTitle': 'Transcription',
        'config.transcriptionDesc': 'Audio transcription settings',
        'config.transcriptionModel': 'Model',
        'config.transcriptionModelPlaceholder': 'whisper-1',
        'config.transcriptionBaseUrl': 'Base URL',
        'config.transcriptionBaseUrlHint': 'Leave empty for default',
        'config.transcriptionBaseUrlPlaceholder': 'https://api.openai.com/v1',
        'config.transcriptionLanguage': 'Language',
        'config.transcriptionLanguageHint': 'Leave empty for auto-detect',
        'config.transcriptionApiKey': 'API Key',
        'config.transcriptionApiKeyHint': 'Uses main key if empty',
        'config.transcriptionApiKeyPlaceholder': 'Enter key',
        'config.apiKeyConfigured': 'configured',
        'config.autoDetect': 'Auto-detect',
        'apiConfig.endpoint': 'Endpoint',
        'apiConfig.baseUrl': 'Base URL',
        'apiConfig.baseUrlHint': 'Leave empty to use default',
        'apiConfig.apiKey': 'API Key',
        'apiConfig.apiKeyPlaceholder': 'Enter your API key',
        'apiConfig.apiKeyHint': 'Encrypted and stored in vault',
        'apiConfig.model': 'Model',
        'apiConfig.selectModel': 'Select a model',
        'apiConfig.selectOrTypeModel': 'Select or type a model',
        'apiConfig.testConnection': 'Test Connection',
        'apiConfig.testing': 'Testing...',
        'apiConfig.connectionFailed': 'Connection failed',
        'apiConfig.getApiKey': 'Get API Key',
        'apiConfig.context1m': 'Enable 1M Context',
        'apiConfig.context1mDesc': 'Enable 1M token context beta',
        'apiConfig.toolStream': 'Real-time Tool Streaming',
        'apiConfig.toolStreamDesc': 'Enable streaming tool calls',
        'apiConfig.validation.providerRequired': 'Please select a provider',
        'apiConfig.validation.modelRequired': 'Please select a model',
        'apiConfig.validation.apiKeyRequired': 'API key required',
        'common.backToSettings': 'Back',
        'unsavedChanges.message': 'You have unsaved changes',
        'unsavedChanges.save': 'Save',
        'unsavedChanges.discard': 'Discard',
        'unsavedChanges.saved': 'Saved',
        'unsavedChanges.error': 'Error saving',
        'unsavedChanges.retry': 'Retry',
        'setupPage.modelName': 'Model name',
      }
      return translations[key] || key
    },
  }),
}))

// Mock useAppStore (used by UnsavedChangesBar)
vi.mock('@/stores/app', () => ({
  useAppStore: (selector: (s: Record<string, unknown>) => unknown) =>
    selector({ sidebarOpen: false, sidebarCollapsed: false }),
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
  media: {
    vision_enabled: false,
    vision_model: '',
    vision_detail: 'auto',
    transcription_enabled: false,
    transcription_model: '',
    transcription_base_url: '',
    transcription_api_key: false,
    transcription_language: '',
  },
}

function renderApiConfig() {
  return render(
    <MemoryRouter>
      <ApiConfig />
    </MemoryRouter>
  )
}

describe('ApiConfig', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(apiModule.api.config.get).mockResolvedValue(mockConfig as unknown as Record<string, unknown>)
    vi.mocked(apiModule.api.config.update).mockResolvedValue(mockConfig as unknown as Record<string, unknown>)
    vi.mocked(apiModule.api.setup.testProvider).mockResolvedValue({ success: true })
  })

  it('should render loading state initially', () => {
    renderApiConfig()

    const spinner = document.querySelector('.animate-spin')
    expect(spinner).toBeInTheDocument()
  })

  it('should render provider cards after loading', async () => {
    renderApiConfig()

    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument()
    })

    expect(screen.getByText('Anthropic')).toBeInTheDocument()
    expect(screen.getByText('Google')).toBeInTheDocument()
  })

  it('should select provider when clicked', async () => {
    renderApiConfig()

    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument()
    })

    await userEvent.click(screen.getByText('Anthropic'))

    const anthropicCard = screen.getByText('Anthropic').closest('button')
    expect(anthropicCard).toBeInTheDocument()
  })

  it('should test connection when test button is clicked', async () => {
    renderApiConfig()

    await waitFor(() => {
      expect(screen.getByText('Test Connection')).toBeInTheDocument()
    })

    await userEvent.click(screen.getByText('Test Connection'))

    await waitFor(() => {
      expect(apiModule.api.setup.testProvider).toHaveBeenCalled()
    })
  })

  it('should show unsaved changes bar and save when clicked', async () => {
    renderApiConfig()

    await waitFor(() => {
      expect(screen.getByText('API Key')).toBeInTheDocument()
    })

    // Enter a new API key to trigger a change
    const passwordInput = document.querySelector('input[type="password"]') as HTMLInputElement
    expect(passwordInput).toBeInTheDocument()
    await userEvent.type(passwordInput, 'test-api-key')

    // The unsaved changes bar should appear
    await waitFor(() => {
      expect(screen.getByText('You have unsaved changes')).toBeInTheDocument()
    })

    // Click Save in the unsaved changes bar
    const saveButton = screen.getByText('Save')
    await userEvent.click(saveButton)

    await waitFor(() => {
      expect(apiModule.api.config.update).toHaveBeenCalled()
    }, { timeout: 3000 })
  })

  it('should show API Key field for providers that require it', async () => {
    renderApiConfig()

    await waitFor(() => {
      expect(screen.getByText('API Key')).toBeInTheDocument()
    })

    const passwordInput = document.querySelector('input[type="password"]')
    expect(passwordInput).toBeInTheDocument()
  })

  it('should show Model field', async () => {
    renderApiConfig()

    await waitFor(() => {
      expect(screen.getByText('Model')).toBeInTheDocument()
    })
  })

  it('should show "See More" card for expanding provider grid', async () => {
    renderApiConfig()

    await waitFor(() => {
      expect(screen.getByText('See More')).toBeInTheDocument()
    })

    // Click "See More" to expand
    await userEvent.click(screen.getByText('See More'))

    // After expanding, "See More" should disappear and more providers should be visible
    await waitFor(() => {
      expect(screen.queryByText('See More')).not.toBeInTheDocument()
    })
  })

  it('should show Vision and Transcription sections', async () => {
    renderApiConfig()

    await waitFor(() => {
      expect(screen.getByText('Vision')).toBeInTheDocument()
    })

    expect(screen.getByText('Transcription')).toBeInTheDocument()
  })
})
