<?php

namespace FlowCatalyst\Generated\Endpoint;

class GetFieldWorkerPhoto extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $id;
    /**
     * image/jpeg; size=thumb (128 px, the default) or full (512 px). ETag is "<version>-<size>", Cache-Control is private, no-cache, so a removal or a lost permission takes effect at once; send If-None-Match for a 304.
     * @param string $id
     * @param array{
     *    "size"?: string, //thumb (128 px) or full (512 px)
     * } $queryParameters
     * @param array{
     *    "If-None-Match"?: string, //An ETag from an earlier answer; 304 when the photo is unchanged
     * } $headerParameters
     */
    public function __construct(string $id, array $queryParameters = [], array $headerParameters = [])
    {
        $this->id = $id;
        $this->queryParameters = $queryParameters;
        $this->headerParameters = $headerParameters;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'GET';
    }
    public function getUri(): string
    {
        return str_replace(['{id}'], [rawurlencode($this->id)], '/api/field-workers/{id}/photo');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['image/jpeg']];
    }
    protected function getQueryOptionsResolver(): \Symfony\Component\OptionsResolver\OptionsResolver
    {
        $optionsResolver = parent::getQueryOptionsResolver();
        $optionsResolver->setDefined(['size']);
        $optionsResolver->setRequired([]);
        $optionsResolver->setDefaults(['size' => 'thumb']);
        $optionsResolver->addAllowedTypes('size', ['string']);
        return $optionsResolver;
    }
    protected function getHeadersOptionsResolver(): \Symfony\Component\OptionsResolver\OptionsResolver
    {
        $optionsResolver = parent::getHeadersOptionsResolver();
        $optionsResolver->setDefined(['If-None-Match']);
        $optionsResolver->setRequired([]);
        $optionsResolver->setDefaults([]);
        $optionsResolver->addAllowedTypes('If-None-Match', ['string']);
        return $optionsResolver;
    }
    protected function getQueryStyles(): array
    {
        return ['size' => ['style' => 'form', 'explode' => false]];
    }
    /**
     * {@inheritdoc}
     *
     *
     * @return null
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (200 === $status) {
        }
        if (304 === $status) {
            return null;
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}