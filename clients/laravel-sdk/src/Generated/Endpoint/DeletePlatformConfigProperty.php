<?php

namespace FlowCatalyst\Generated\Endpoint;

class DeletePlatformConfigProperty extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $app;
    protected $section;
    protected $property;
    /**
     * @param string $app
     * @param string $section
     * @param string $property
     * @param array{
     *    "clientId"?: string, //Scope to one client: the client's id (clt_...) or its identifier (e.g. acme). The platform stores the id, so both name the same value. Omit for the application's global value. An unknown client is 404.
     * } $queryParameters
     * @param array{
     *    "X-On-Behalf-Of"?: string, //Who the calling service account acts for (for example the application's signed-in user), recorded in the platform's audit row. Honoured only from a service account; ignored from anyone else. At most 256 printable characters.
     * } $headerParameters
     */
    public function __construct(string $app, string $section, string $property, array $queryParameters = [], array $headerParameters = [])
    {
        $this->app = $app;
        $this->section = $section;
        $this->property = $property;
        $this->queryParameters = $queryParameters;
        $this->headerParameters = $headerParameters;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'DELETE';
    }
    public function getUri(): string
    {
        return str_replace(['{app}', '{section}', '{property}'], [$this->app, $this->section, $this->property], '/api/config/{app}/{section}/{property}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    protected function getQueryOptionsResolver(): \Symfony\Component\OptionsResolver\OptionsResolver
    {
        $optionsResolver = parent::getQueryOptionsResolver();
        $optionsResolver->setDefined(['clientId']);
        $optionsResolver->setRequired([]);
        $optionsResolver->setDefaults([]);
        $optionsResolver->addAllowedTypes('clientId', ['string']);
        return $optionsResolver;
    }
    protected function getHeadersOptionsResolver(): \Symfony\Component\OptionsResolver\OptionsResolver
    {
        $optionsResolver = parent::getHeadersOptionsResolver();
        $optionsResolver->setDefined(['X-On-Behalf-Of']);
        $optionsResolver->setRequired([]);
        $optionsResolver->setDefaults([]);
        $optionsResolver->addAllowedTypes('X-On-Behalf-Of', ['string']);
        return $optionsResolver;
    }
    /**
     * {@inheritdoc}
     *
     *
     * @return null|\FlowCatalyst\Generated\Model\ErrorModel
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (204 === $status) {
            return null;
        }
        if (mb_strpos(strtolower($contentType), 'application/json') !== false) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ErrorModel', 'json');
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}