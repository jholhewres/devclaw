import type { FC } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Button as AriaButton } from 'react-aria-components';
import { Tooltip } from '@/components/base/tooltip/tooltip';
import { cx } from '@/utils/cx';
import { useAppStore } from '@/stores/app';

export interface NavItemData {
  nameKey: string;
  icon: FC<{ className?: string }>;
  route: string;
  activeRoutes?: string[];
  onClick?: () => void;
}

interface NavItemProps {
  item: NavItemData;
  collapsed?: boolean;
  iconClassName?: string;
  labelClassName?: string;
}

export function NavItem({ item, collapsed, iconClassName, labelClassName }: NavItemProps) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const location = useLocation();
  const setSidebarOpen = useAppStore((s) => s.setSidebarOpen);

  const Icon = item.icon;
  const label = t(item.nameKey);
  const active = isActive(location.pathname, item.route, item.activeRoutes);

  const handlePress = () => {
    if (item.onClick) {
      item.onClick();
    } else {
      navigate(item.route);
      setSidebarOpen(false);
    }
  };

  const button = (
    <AriaButton
      onPress={handlePress}
      className={cx(
        'group relative flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm font-semibold outline-hidden transition-all duration-150 cursor-pointer',
        active
          ? 'bg-active text-secondary_hover'
          : 'text-fg-quaternary pressed:bg-active hover:bg-primary_hover',
        collapsed && 'justify-center',
      )}
    >
      <Icon
        className={cx(
          'size-5 shrink-0',
          active ? 'text-fg-brand-secondary' : 'text-fg-quaternary group-hover:text-fg-tertiary',
          iconClassName,
        )}
      />
      {!collapsed && <span className={cx('truncate', labelClassName)}>{label}</span>}
    </AriaButton>
  );

  if (collapsed) {
    return (
      <Tooltip title={label} placement="right" delay={200}>
        {button}
      </Tooltip>
    );
  }

  return button;
}

function isActive(pathname: string, route: string, extra?: string[]) {
  if (pathname === route) return true;
  if (route !== '/' && pathname.startsWith(route)) return true;
  if (extra?.some((r) => pathname.startsWith(r))) return true;
  return false;
}
