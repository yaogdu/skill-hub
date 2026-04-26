"use client"

import { Github } from "lucide-react"

export function Footer() {
  return (
    <footer className="mt-auto border-t bg-background/80 backdrop-blur">
      <div className="container mx-auto flex flex-col gap-4 px-6 py-6 text-sm text-muted-foreground md:flex-row md:items-center md:justify-between">
        <div>
          <p className="font-medium text-foreground">skill-hub</p>
          <p>Open-source skill registry UI, built on top of the agentregistry codebase.</p>
        </div>

        <div className="flex items-center gap-4">
          <a
            href="https://github.com/yaogdu/skill-hub"
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-1.5 transition-colors hover:text-foreground"
          >
            <Github className="h-4 w-4" />
            GitHub repo
          </a>
        </div>
      </div>
    </footer>
  )
}
