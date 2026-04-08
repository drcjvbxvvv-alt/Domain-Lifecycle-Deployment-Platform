<script setup lang="ts">
import { NConfigProvider, NGlobalStyle, darkTheme } from 'naive-ui'
import { RouterView } from 'vue-router'
</script>

<template>
  <NConfigProvider :theme="darkTheme">
    <NGlobalStyle />
    <RouterView />
  </NConfigProvider>
</template>
