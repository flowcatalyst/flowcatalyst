<?php

namespace FlowCatalyst\Generated\Endpoint;

class GetApiAdminConfigByAppCodeBySectionByProperty extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $appCode;
    protected $property;
    protected $section;
    /**
     * @param string $appCode
     * @param string $property
     * @param string $section
     * @param array{
     *    "clientId"?: string,
     *    "scope"?: string,
     * } $queryParameters
     */
    public function __construct(string $appCode, string $property, string $section, array $queryParameters = [])
    {
        $this->appCode = $appCode;
        $this->property = $property;
        $this->section = $section;
        $this->queryParameters = $queryParameters;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'GET';
    }
    public function getUri(): string
    {
        return str_replace(['{appCode}', '{property}', '{section}'], [$this->appCode, $this->property, $this->section], '/api/admin/config/{appCode}/{section}/{property}');
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
        $optionsResolver->setDefined(['clientId', 'scope']);
        $optionsResolver->setRequired([]);
        $optionsResolver->setDefaults([]);
        $optionsResolver->addAllowedTypes('clientId', ['string']);
        $optionsResolver->addAllowedTypes('scope', ['string']);
        return $optionsResolver;
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminConfigByAppCodeBySectionByPropertyForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminConfigByAppCodeBySectionByPropertyNotFoundException
     *
     * @return null
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return json_decode($body);
        }
        if (403 === $status) {
            throw new \FlowCatalyst\Generated\Exception\GetApiAdminConfigByAppCodeBySectionByPropertyForbiddenException($response);
        }
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\GetApiAdminConfigByAppCodeBySectionByPropertyNotFoundException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}