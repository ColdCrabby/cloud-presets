import { Routes } from '@angular/router';

export const routes: Routes = [
  {
    path: '',
    title: 'Vendor dashboard',
    loadComponent: () => import('./pages/dashboard/dashboard').then((m) => m.Dashboard),
  },
];
