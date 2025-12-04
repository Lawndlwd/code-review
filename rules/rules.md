# React & TypeScript Guidelines

## Naming Conventions

- **Components**: PascalCase (`UserProfile`, `DataTable`)
- **Files**: PascalCase for components (`UserProfile.tsx`), camelCase for utilities (`formatDate.ts`)
- **Hooks**: camelCase with `use` prefix (`useUserData`, `useFormState`)
- **Constants**: UPPER_SNAKE_CASE (`API_ENDPOINT`, `MAX_RETRIES`)
- **Variables/Functions**: camelCase (`userData`, `handleSubmit`)
- **Types/Interfaces**: PascalCase with `type` keyword (`type User = {}`)
- **Props Types**: ComponentName + `Props` (`type UserProfileProps = {}`)
- **Event Handlers**: `handle` prefix (`handleClick`, `handleInputChange`)
- **Boolean Variables**: `is`, `has`, `should` prefix (`isLoading`, `hasError`, `shouldRender`)
- don't suggest changes for imported variables or functions or types inside the imported file if you have suggestion should be in the definition file not who import it 

## Component Structure

- Always use functional components with hooks
- Prefer named exports over default exports
- Destructure props in function signature: `({ name, age }: UserProps)`
- Define event handlers outside JSX elements
- Group related state with `useReducer` for complex state logic

## State Management

- Use `useState` for simple local state
- Use `useReducer` for complex state with multiple sub-values
- Use `useSuspenseQuery` for data fetching with Suspense
- Use `useDeferredValue` for non-urgent updates
- Prefer derived state over duplicate state
- Lift state up when shared between components

## Data Fetching

- Use `@scaleway/use-dataloader` for data fetching
- Use `@shire/queries` for list and get queries
- Handle loading and error states explicitly (unless for List pages or Table or a component that handle loading by passing in props like isLoading={isLoading && !data})
- Use Suspense boundaries for async components

## Internationalization
(don't check locales.ts files)
- Never hardcode text strings in components (not in locales)
- Always use `@scaleway/use-i18n` for all user-facing (empty string/ numbers or any value don't need like ('{}') translations don't count)
- Define translations in locale files
- Use interpolation for dynamic values: `scopedT('welcome', { name: userName })`

## Styling

- Use Ultraviolet components (`@ultraviolet/ui`) as primary UI building blocks or @shire/ui
- Avoid custom CSS or styled components
- Use component props for styling variations
- Use `&nbsp;` or CSS styling for spacing, never `{' '}`

## Conditional Rendering

- Use explicit ternary with `null`: `{isValid ? <Component /> : null}`
- Avoid `&&` operator for conditional rendering
- Extract complex conditions to variables: `const shouldShow = isValid && hasPermission`

## Props & TypeScript

- Define prop types using `type` keyword
- Make all props explicitly typed, no implicit `any`
- Use optional props sparingly: `name?: string`
- Destructure props with types: `({ name, age }: Props)`
- Use `React.ReactNode` for children: `children: ReactNode`
- it is ok to initialized to an empty string/ null or undefined in intial values 

## Event Handlers

- Define handlers outside JSX: `const handleClick = () => {}`
- Type event parameters: `(e: React.MouseEvent<HTMLButtonElement>)`
- Avoid inline arrow functions in JSX props
- Use `useCallback` for handlers passed to memoized children

## Forms

- Use `@ultraviolet/form` for form handling
- Define validation schemas with form library
- Handle form submission with typed handlers
- Use controlled components for form inputs

## Performance

- Use `React.memo` for expensive components
- Use `useMemo` for expensive computations
- Use `useCallback` for functions passed to children
- Avoid creating objects/arrays in render: `const style = useMemo(() => ({ ... }), [])`
- Use `useDeferredValue` for non-critical updates

## Imports

- Use type imports: `import type { User } from './types'`
- Use absolute imports for shared code
- Never import mocks in non-test files (__mocks__, */mocks/**)

## Test Mocks

- Mock files (from `__mocks__/**, */mocks/**` where ** means any thing after or before) should ONLY be imported in test files (`*.test.ts`, `*.spec.ts`)
- Never import mocks in production code

## Data Structures

- Use destructuring: `const { name, age } = user`
- Use spread operator: `{ ...user, active: true }`
- Use optional chaining: `user?.profile?.name`
- Use nullish coalescing: `name ?? 'Unknown'`

## Arrays & Objects

- Use `.map()` for transformations, always include `key` prop
- Use `.filter()` for filtering
- Use `.find()` for single item lookup
- Avoid `.forEach()`, prefer `.map()` or `.reduce()`
- Use immutable operations: `[...items, newItem]`

## React Router

- Use React Router DOM 5.3.4 conventions
- Define routes declaratively
- Use hooks for navigation: `useHistory`, `useParams`, `useLocation`
- Type route params: `type RouteParams = { id: string }`

## Analytics

- Use `@scaleway/use-analytics` for tracking
- Use `@scaleway/use-growthbook` for feature flags
- Track user actions explicitly
- Never track sensitive data

## SDK Integration

- Use `@scaleway-internal/sdk-*` packages for API calls
- Type all SDK responses
- Handle SDK errors with error boundaries
- Use SDK hooks when available

## Error Handling

- Use Error Boundaries for component errors
- Handle async errors in data fetching hooks
- Display user-friendly error messages using i18n
- Log errors for debugging: `console.error(error)`

## Accessibility

- Use semantic HTML elements
- Include ARIA labels when needed
- Ensure keyboard navigation works
- Use Ultraviolet accessible components
- Test with screen readers

## Testing

- Keep test files separate: `*.test.tsx`, `*.spec.tsx`
- Use React Testing Library conventions
- Mock external dependencies
- Test user interactions, not implementation

## Code Organization

- One component per file
- Co-locate related files (component, styles, types)
- Extract reusable logic to custom hooks
- Keep components small and focused
- Separate business logic from presentation

## Comments

- Use TSDoc for public APIs: `/** Description */`
- Avoid obvious comments
- Explain "why", not "what"
- Keep comments up-to-date with code

## Avoid

- `any` type without explicit reason
- Non-null assertions (`!`) without safety checks
- Inline styles objects in render
- Mutating props or state directly
- Deeply nested ternaries
- Large components (>300 lines)
- Mixing concerns in single component
- Hardcoded strings
- Magic numbers without constants
- Unused imports or variables

## Code Review Guidelines - Avoiding Redundant Suggestions

- **Avoid Suggesting Changes Already Handled:** Do not suggest changes that are already accounted for in the code. For example, if a function explicitly handles `undefined` values, do not suggest adding optional chaining (`?.`) to prevent errors.
- **Optional Props:**  Optional props in types should only be used when genuinely needed based on business logic. Do not make a prop optional simply to avoid a type error if the function already handles the `undefined` case. Ensure consistency between the type definition and the actual usage of the prop.
- **Focus on Logic, Not Just Types:** Prioritize reviewing the underlying logic and functionality over strict type adherence if the code already functions correctly with the existing types.

- it is ok that prop was changed from required to optional without clear justification

Unhelpful Suggestions

- **Optional Props & Optional Chaining:** Do not flag optional props used with optional chaining (`?.`) as an issue unless there's a clear indication that the prop is *not* being handled correctly (e.g., potential for errors if the prop is missing).  The use of optional chaining inherently addresses the possibility of an undefined value.
- **Contextual Analysis:** Prioritize understanding the *intent* of the code and the surrounding context before flagging potential issues. Avoid surface-level checks that don't consider the broader picture.
- **Avoid Noise:** Focus on meaningful issues that impact correctness, performance, or maintainability.  Do not generate "noise" with trivial or already-addressed concerns.
