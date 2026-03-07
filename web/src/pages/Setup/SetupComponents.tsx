import { useState, type ReactNode, type FC } from 'react'
import { Eye, EyeOff } from 'lucide-react'
import { cn } from '@/lib/utils'

/* ─────────────────────────────────────────────────────────────
   Layout Components
   ───────────────────────────────────────────────────────────── */

export function StepContainer({ children }: { children: ReactNode }) {
  return <div className="space-y-5">{children}</div>
}

export function StepHeader({ title, description }: { title: string; description: string }) {
  return (
    <div>
      <h2 className="text-base font-semibold text-text-primary">{title}</h2>
      <p className="mt-1 text-sm text-text-secondary">{description}</p>
    </div>
  )
}

export function FieldGroup({ children }: { children: ReactNode }) {
  return <div className="space-y-4">{children}</div>
}

/* ─────────────────────────────────────────────────────────────
   Field & Label
   ───────────────────────────────────────────────────────────── */

interface FieldProps {
  label: string
  icon?: FC<{ className?: string }>
  hint?: string
  children: ReactNode
}

export function Field({ label, icon: Icon, hint, children }: FieldProps) {
  return (
    <div>
      <label className="mb-1.5 flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-text-muted">
        {Icon && <Icon className="h-3.5 w-3.5" />}
        {label}
      </label>
      {children}
      {hint && <p className="mt-1.5 text-xs text-text-muted">{hint}</p>}
    </div>
  )
}

/* ─────────────────────────────────────────────────────────────
   Input Components
   ───────────────────────────────────────────────────────────── */

interface InputProps {
  value: string
  onChange: (value: string) => void
  placeholder?: string
  type?: 'text' | 'password' | 'tel' | 'url'
  mono?: boolean
  className?: string
}

export function Input({
  value,
  onChange,
  placeholder,
  type = 'text',
  mono,
  className = '',
}: InputProps) {
  return (
    <input
      type={type}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      autoComplete="off"
      data-lpignore="true"
      data-form-type="other"
      className={cn(
        'h-11 w-full rounded-xl border border-border-hover bg-bg-main px-4 text-sm text-text-primary',
        'placeholder:text-text-muted outline-none transition-all',
        'hover:border-border-hover focus:border-brand/50 focus:ring-2 focus:ring-brand/20',
        mono && 'font-mono',
        className,
      )}
    />
  )
}

interface PasswordInputProps {
  value: string
  onChange: (value: string) => void
  placeholder?: string
}

export function PasswordInput({ value, onChange, placeholder }: PasswordInputProps) {
  const [show, setShow] = useState(false)
  const [focused, setFocused] = useState(false)

  return (
    <div className="relative">
      <input
        type={show ? 'text' : 'password'}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onFocus={() => setFocused(true)}
        placeholder={placeholder}
        autoComplete="new-password"
        name="devclaw-new-password"
        id="devclaw-new-password"
        data-lpignore="true"
        data-form-type="other"
        data-1p-ignore=""
        readOnly={!focused}
        className={cn(
          'h-11 w-full rounded-xl border border-border-hover bg-bg-main px-4 pr-10 text-sm text-text-primary',
          'placeholder:text-text-muted outline-none transition-all',
          'hover:border-border-hover focus:border-brand/50 focus:ring-2 focus:ring-brand/20',
        )}
      />
      <button
        type="button"
        onMouseDown={(e) => { e.preventDefault(); setShow(!show) }}
        className="absolute right-3 top-1/2 -translate-y-1/2 cursor-pointer text-text-muted transition-colors hover:text-text-primary"
      >
        {show ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
      </button>
    </div>
  )
}

interface SelectProps {
  value: string
  onChange: (value: string) => void
  placeholder?: string
  options?: { value: string; label: string }[]
  groups?: { label: string; options: { value: string; label: string }[] }[]
}

export function Select({ value, onChange, placeholder, options = [], groups }: SelectProps) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className={cn(
        'h-11 w-full cursor-pointer rounded-xl border border-border-hover bg-bg-main px-4 text-sm text-text-primary',
        'outline-none transition-all',
        'hover:border-border-hover focus:border-brand/50 focus:ring-2 focus:ring-brand/20',
      )}
    >
      {placeholder && <option value="">{placeholder}</option>}
      {options.map((opt) => (
        <option key={opt.value} value={opt.value}>{opt.label}</option>
      ))}
      {groups?.map((group) => (
        <optgroup key={group.label} label={group.label}>
          {group.options.map((opt) => (
            <option key={opt.value} value={opt.value}>{opt.label}</option>
          ))}
        </optgroup>
      ))}
    </select>
  )
}

/* ─────────────────────────────────────────────────────────────
   Card Components
   ───────────────────────────────────────────────────────────── */

interface CardProps {
  children: ReactNode
  className?: string
  highlight?: 'blue' | 'green' | 'amber' | 'red'
}

export function Card({ children, className = '', highlight }: CardProps) {
  const highlightStyles = {
    blue: 'border-brand/30 bg-brand-subtle',
    green: 'border-success/30 bg-success-subtle',
    amber: 'border-warning/30 bg-warning-subtle',
    red: 'border-error/30 bg-error-subtle',
  }

  return (
    <div className={cn(
      'rounded-xl border p-4 transition-all',
      highlight
        ? highlightStyles[highlight]
        : 'border-border bg-bg-main/50',
      className,
    )}>
      {children}
    </div>
  )
}

interface SelectableCardProps {
  selected: boolean
  onClick: () => void
  icon?: FC<{ className?: string }>
  iconColor?: string
  title: string
  description?: string
  accentColor?: string
}

export function SelectableCard({
  selected,
  onClick,
  icon: Icon,
  iconColor,
  title,
  description,
  accentColor,
}: SelectableCardProps) {
  return (
    <button
      onClick={onClick}
      className={cn(
        'flex w-full cursor-pointer items-start gap-3 rounded-xl border px-4 py-3 text-left transition-all',
        selected
          ? 'border-brand/50 bg-brand-subtle'
          : 'border-border bg-bg-main/50 hover:border-border-hover hover:bg-bg-surface',
      )}
      style={selected && accentColor ? { borderColor: accentColor, backgroundColor: `${accentColor}15` } : undefined}
    >
      {Icon && (
        <div className={cn(
          'mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-lg',
          selected ? 'bg-white/5' : 'bg-bg-subtle',
        )}>
          <Icon className={cn(
            'h-3.5 w-3.5',
            selected ? (iconColor || 'text-brand') : 'text-text-muted',
          )} />
        </div>
      )}
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-text-primary">{title}</span>
          {selected && <span className="h-1.5 w-1.5 rounded-full bg-brand" />}
        </div>
        {description && <p className="mt-0.5 text-xs text-text-secondary">{description}</p>}
      </div>
    </button>
  )
}

/* ─────────────────────────────────────────────────────────────
   Toggle & Checkbox
   ───────────────────────────────────────────────────────────── */

interface ToggleProps {
  enabled: boolean
  onChange: (enabled: boolean) => void
  label?: string
}

export function Toggle({ enabled, onChange, label }: ToggleProps) {
  return (
    <button
      type="button"
      onClick={() => onChange(!enabled)}
      className="flex cursor-pointer items-center gap-2"
    >
      <div className={cn(
        'relative h-5 w-9 rounded-full transition-colors',
        enabled ? 'bg-brand' : 'bg-bg-subtle',
      )}>
        <span className={cn(
          'absolute top-0.5 h-4 w-4 rounded-full bg-white shadow transition-transform',
          enabled ? 'translate-x-4' : 'translate-x-0.5',
        )} />
      </div>
      {label && <span className="text-xs text-text-secondary">{label}</span>}
    </button>
  )
}

interface CheckboxProps {
  checked: boolean
  onChange: () => void
  children: ReactNode
}

export function Checkbox({ checked, onChange, children }: CheckboxProps) {
  return (
    <button onClick={onChange} className="flex cursor-pointer items-center gap-2.5">
      <div className={cn(
        'flex h-5 w-5 shrink-0 items-center justify-center rounded border transition-all',
        checked
          ? 'border-transparent bg-brand text-white'
          : 'border-border-hover bg-bg-subtle hover:border-text-muted',
      )}>
        {checked && (
          <svg className="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
          </svg>
        )}
      </div>
      {children}
    </button>
  )
}

/* ─────────────────────────────────────────────────────────────
   Button Components
   ───────────────────────────────────────────────────────────── */

interface ButtonProps {
  children: ReactNode
  onClick: () => void
  disabled?: boolean
  loading?: boolean
  variant?: 'primary' | 'secondary' | 'ghost'
  size?: 'sm' | 'md'
  icon?: FC<{ className?: string }>
}

export function Button({
  children,
  onClick,
  disabled,
  loading,
  variant = 'secondary',
  size = 'md',
  icon: Icon,
}: ButtonProps) {
  const variants = {
    primary: 'bg-text-primary text-text-inverse shadow-lg hover:bg-white',
    secondary: 'border border-border-hover bg-bg-subtle text-text-primary hover:border-text-muted hover:bg-bg-elevated',
    ghost: 'text-text-muted hover:text-text-primary',
  }

  const sizes = {
    sm: 'px-3 py-2 text-xs',
    md: 'px-4 py-2.5 text-sm',
  }

  return (
    <button
      onClick={onClick}
      disabled={disabled || loading}
      className={cn(
        'flex cursor-pointer items-center justify-center gap-2 rounded-xl font-medium transition-all',
        'disabled:cursor-not-allowed disabled:opacity-40',
        variants[variant],
        sizes[size],
      )}
    >
      {loading ? (
        <div className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-current/30 border-t-current" />
      ) : Icon ? (
        <Icon className="h-3.5 w-3.5" />
      ) : null}
      {children}
    </button>
  )
}

/* ─────────────────────────────────────────────────────────────
   Button Group / Grid Select
   ───────────────────────────────────────────────────────────── */

interface OptionButtonProps {
  selected: boolean
  onClick: () => void
  children: ReactNode
  className?: string
}

export function OptionButton({ selected, onClick, children, className = '' }: OptionButtonProps) {
  return (
    <button
      onClick={onClick}
      className={cn(
        'flex cursor-pointer items-center gap-2 rounded-xl border px-3 py-2.5 text-left transition-all',
        selected
          ? 'border-brand/50 bg-brand-subtle text-text-primary'
          : 'border-border-hover bg-bg-main text-text-secondary hover:border-text-muted hover:bg-bg-surface',
        className,
      )}
    >
      {children}
    </button>
  )
}

/* ─────────────────────────────────────────────────────────────
   Info Box
   ───────────────────────────────────────────────────────────── */

interface InfoBoxProps {
  icon?: FC<{ className?: string }>
  children: ReactNode
}

export function InfoBox({ icon: Icon, children }: InfoBoxProps) {
  return (
    <div className="flex items-start gap-2.5 rounded-xl border border-border bg-bg-main/50 px-4 py-3">
      {Icon && (
        <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-bg-subtle">
          <Icon className="h-3.5 w-3.5 text-text-muted" />
        </div>
      )}
      <p className="text-xs text-text-secondary">{children}</p>
    </div>
  )
}
