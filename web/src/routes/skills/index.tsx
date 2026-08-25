/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { createFileRoute, redirect } from '@tanstack/react-router'

import { Skills } from '@/features/skills'
import { getFreshModuleAccess } from '@/lib/nav-modules'

export const Route = createFileRoute('/skills/')({
  beforeLoad: async () => {
    const access = await getFreshModuleAccess('skills')
    if (!access.enabled) throw redirect({ to: '/' })
  },
  component: Skills,
})
