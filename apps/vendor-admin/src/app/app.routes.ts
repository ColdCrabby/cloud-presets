import { Routes } from '@angular/router';

export const routes: Routes = [
  {
    path: '',
    title: 'Vendor dashboard',
    loadComponent: () => import('./pages/dashboard/dashboard').then((m) => m.Dashboard),
  },
  {
    path: 'claim/:id',
    title: 'Claim upload',
    loadComponent: () => import('./pages/claim/claim').then((m) => m.Claim),
  },
];
