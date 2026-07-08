import { type FC, useEffect, useState, type FormEvent } from 'react'

interface Props {
    initialQuery?: string
    onSearch: (query: string) => void
}

export const SearchBar: FC<Props> = ({
    initialQuery = '',
    onSearch,
}) => {
    const [value, setValue] = useState(initialQuery)

    useEffect(() => {
        setValue(initialQuery)
    }, [initialQuery])

    const handleSubmit = (e: FormEvent) => {
        e.preventDefault()
        const trimmed = value.trim()
        if (trimmed) {
            onSearch(trimmed)
        }
    }

    return (
        <form className="search-bar" onSubmit={handleSubmit}>
            <input
                type="text"
                value={value}
                onChange={(e) => setValue(e.target.value)}
                placeholder="Search datasets by name, description, or content..."
                autoFocus
            />
            <button type="submit">Search</button>
        </form>
    )
}
