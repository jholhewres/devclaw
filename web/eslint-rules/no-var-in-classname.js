/**
 * Prevent using var(--color-*) in className
 * Use semantic Tailwind classes instead (text-primary, bg-main, etc.)
 */

export default {
  meta: {
    type: 'suggestion',
    docs: {
      description: 'Prevent var(--color-*) in className, use semantic Tailwind classes',
      category: 'Stylistic Issues',
      recommended: false,
    },
    messages: {
      useSemantic: 'Use semantic Tailwind color classes (e.g., text-primary, bg-main) instead of var(--color-...)',
    },
    schema: [],
  },
  create(context) {
    return {
      JSXAttribute(node) {
        if (node.name.name !== 'className') return;

        const value = node.value;
        if (!value || value.type !== 'Literal') return;

        const className = value.value;
        if (!className || typeof className !== 'string') return;

        // Match var(--color-...)
        const varPattern = /var\(--color-[^)]+\)/;
        if (varPattern.test(className)) {
          context.report({
            node,
            messageId: 'useSemantic',
          });
        }
      },
    };
  },
};
