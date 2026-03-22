import type {
  DialogProps as AriaDialogProps,
  ModalOverlayProps as AriaModalOverlayProps,
} from 'react-aria-components';
import {
  Dialog as AriaDialog,
  DialogTrigger as AriaDialogTrigger,
  Modal as AriaModal,
  ModalOverlay as AriaModalOverlay,
} from 'react-aria-components';
import { cx } from '@/utils/cx';

export const DialogTrigger = AriaDialogTrigger;

export const ModalOverlay = (props: AriaModalOverlayProps) => {
  return (
    <AriaModalOverlay
      {...props}
      className={(state) =>
        cx(
          'bg-overlay/70 fixed inset-0 z-50 flex min-h-dvh w-full items-end justify-center overflow-y-auto px-4 pt-4 pb-[clamp(16px,8vh,64px)] outline-hidden backdrop-blur-[6px] sm:items-center sm:justify-center sm:p-8',
          state.isEntering && 'animate-in fade-in duration-300 ease-out',
          state.isExiting && 'animate-out fade-out duration-200 ease-in',
          typeof props.className === 'function' ? props.className(state) : props.className
        )
      }
    />
  );
};

export const Modal = (props: AriaModalOverlayProps) => (
  <AriaModal
    {...props}
    className={(state) =>
      cx(
        'max-h-full w-full align-middle outline-hidden max-sm:overflow-y-auto max-sm:rounded-xl',
        state.isEntering && 'animate-in zoom-in-95 duration-300 ease-out',
        state.isExiting && 'animate-out zoom-out-95 duration-200 ease-in',
        typeof props.className === 'function' ? props.className(state) : props.className
      )
    }
  />
);

export const Dialog = (props: AriaDialogProps) => (
  <AriaDialog
    {...props}
    className={cx('flex w-full items-center justify-center outline-hidden', props.className)}
  />
);
