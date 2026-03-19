import { useState, useRef, useEffect, useCallback, useId } from 'react';
import { useTranslation } from 'react-i18next';
import { ChevronDown } from 'lucide-react';
import { cn } from '@/lib/utils';

interface ModelComboboxProps {
  value: string;
  onChange: (value: string) => void;
  suggestions: string[];
  placeholder?: string;
}

export function ModelCombobox({ value, onChange, suggestions, placeholder }: ModelComboboxProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [activeIndex, setActiveIndex] = useState(-1);
  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLUListElement>(null);
  const listboxId = useId();

  // Split suggestions into popular (no :free) and free (:free)
  const popular = suggestions.filter((m) => !m.endsWith(':free'));
  const free = suggestions.filter((m) => m.endsWith(':free'));

  // Filter by query
  const lowerQuery = query.toLowerCase();
  const filteredPopular = popular.filter((m) => m.toLowerCase().includes(lowerQuery));
  const filteredFree = free.filter((m) => m.toLowerCase().includes(lowerQuery));
  const allFiltered = [...filteredPopular, ...filteredFree];

  // Close on click outside
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  // Scroll active item into view
  useEffect(() => {
    if (activeIndex >= 0 && listRef.current) {
      const items = listRef.current.querySelectorAll('[role="option"]');
      items[activeIndex]?.scrollIntoView({ block: 'nearest' });
    }
  }, [activeIndex]);

  const handleSelect = useCallback(
    (model: string) => {
      onChange(model);
      setQuery('');
      setOpen(false);
      setActiveIndex(-1);
    },
    [onChange],
  );

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value;
    setQuery(val);
    onChange(val);
    setActiveIndex(-1);
    if (!open) setOpen(true);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (!open && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
      setOpen(true);
      e.preventDefault();
      return;
    }

    if (!open) return;

    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        setActiveIndex((prev) => (prev < allFiltered.length - 1 ? prev + 1 : 0));
        break;
      case 'ArrowUp':
        e.preventDefault();
        setActiveIndex((prev) => (prev > 0 ? prev - 1 : allFiltered.length - 1));
        break;
      case 'Enter':
        e.preventDefault();
        if (activeIndex >= 0 && activeIndex < allFiltered.length) {
          handleSelect(allFiltered[activeIndex]);
        } else {
          setOpen(false);
        }
        break;
      case 'Escape':
        e.preventDefault();
        setOpen(false);
        setActiveIndex(-1);
        break;
    }
  };

  const activeOptionId = activeIndex >= 0 ? `${listboxId}-option-${activeIndex}` : undefined;

  return (
    <div ref={containerRef} className="relative">
      {/* Input */}
      <div className="relative">
        <input
          ref={inputRef}
          role="combobox"
          aria-expanded={open}
          aria-haspopup="listbox"
          aria-autocomplete="list"
          aria-controls={listboxId}
          aria-activedescendant={activeOptionId}
          type="text"
          value={query || value}
          onChange={handleInputChange}
          onFocus={() => {
            setOpen(true);
            setQuery('');
          }}
          onKeyDown={handleKeyDown}
          placeholder={placeholder}
          className="h-11 w-full rounded-xl border border-border bg-bg-surface px-4 pr-10 text-sm text-text-primary outline-none transition-all placeholder:text-text-muted hover:border-border-hover focus:border-brand/50 focus:ring-1 focus:ring-brand/20"
        />
        <button
          type="button"
          tabIndex={-1}
          aria-label={t('apiConfig.toggleModelDropdown')}
          onClick={() => {
            setOpen(!open);
            inputRef.current?.focus();
          }}
          className="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted hover:text-text-primary cursor-pointer"
        >
          <ChevronDown
            className={cn('h-4 w-4 transition-transform', open && 'rotate-180')}
          />
        </button>
      </div>

      {/* Dropdown */}
      {open && (
        <ul
          ref={listRef}
          id={listboxId}
          role="listbox"
          className="absolute z-50 mt-1 max-h-64 w-full overflow-auto rounded-xl border border-border bg-bg-surface shadow-lg"
        >
          {/* Popular models */}
          {filteredPopular.length > 0 && (
            <>
              <li role="presentation" className="sticky top-0 z-10 bg-bg-surface px-3 py-1.5 text-[10px] font-semibold uppercase tracking-wider text-text-muted border-b border-border">
                {t('apiConfig.popularModels')}
              </li>
              {filteredPopular.map((model, i) => {
                const flatIdx = i;
                return (
                  <li
                    key={model}
                    id={`${listboxId}-option-${flatIdx}`}
                    role="option"
                    aria-selected={flatIdx === activeIndex}
                    onClick={() => handleSelect(model)}
                    className={cn(
                      'cursor-pointer px-4 py-2 text-sm text-text-primary transition-colors',
                      flatIdx === activeIndex
                        ? 'bg-brand/10 text-brand'
                        : 'hover:bg-bg-subtle',
                      value === model && 'font-medium',
                    )}
                  >
                    {model}
                  </li>
                );
              })}
            </>
          )}

          {/* Free models */}
          {filteredFree.length > 0 && (
            <>
              <li role="presentation" className="sticky top-0 z-10 bg-bg-surface px-3 py-1.5 text-[10px] font-semibold uppercase tracking-wider text-text-muted border-b border-border">
                {t('apiConfig.freeModels')}
              </li>
              {filteredFree.map((model, i) => {
                const flatIdx = filteredPopular.length + i;
                return (
                  <li
                    key={model}
                    id={`${listboxId}-option-${flatIdx}`}
                    role="option"
                    aria-selected={flatIdx === activeIndex}
                    onClick={() => handleSelect(model)}
                    className={cn(
                      'cursor-pointer px-4 py-2 text-sm text-text-primary transition-colors',
                      flatIdx === activeIndex
                        ? 'bg-brand/10 text-brand'
                        : 'hover:bg-bg-subtle',
                      value === model && 'font-medium',
                    )}
                  >
                    {model}
                  </li>
                );
              })}
            </>
          )}

          {/* No results */}
          {allFiltered.length === 0 && (
            <li role="presentation" className="px-4 py-3 text-sm text-text-muted italic">
              {t('apiConfig.modelComboboxHint')}
            </li>
          )}

          {/* Hint footer */}
          {allFiltered.length > 0 && (
            <li role="presentation" className="border-t border-border px-3 py-2 text-[10px] text-text-muted">
              {t('apiConfig.modelComboboxHint')}
            </li>
          )}
        </ul>
      )}
    </div>
  );
}
